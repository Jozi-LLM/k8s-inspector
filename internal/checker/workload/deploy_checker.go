// internal/checker/workload/deployment_checker.go
package workload

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

type DeploymentChecker struct{}

func (c *DeploymentChecker) Name() string {
	return "deployment_health"
}

func (c *DeploymentChecker) Description() string {
	return "检查Deployment状态、副本数和更新策略"
}

func (c *DeploymentChecker) Category() string {
	return "workload"
}

func (c *DeploymentChecker) RequiredPermissions() []string {
	return []string{"get", "list", "watch"}
}

func (c *DeploymentChecker) Execute(ctx context.Context, client *k8s.Client) ([]types.CheckDetail, error) {
	logger := utils.GetGlobalLogger()
	logger.Infow("开始执行Deployment检查", "checker", c.Name())

	startTime := time.Now()
	var results []types.CheckDetail

	// 获取所有命名空间的Deployment
	namespaces, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "namespace_list",
			Status:    types.StatusError,
			Message:   fmt.Sprintf("获取命名空间列表失败: %v", err),
			Severity:  types.SeverityMedium,
			Timestamp: time.Now(),
		})
		return results, nil
	}

	totalDeployments := 0
	unhealthyDeployments := 0

	// 检查每个命名空间
	for _, ns := range namespaces.Items {
		namespace := ns.Name

		// 跳过系统命名空间
		if strings.HasPrefix(namespace, "kube-") {
			continue
		}

		deployments, err := client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   fmt.Sprintf("deployment_list_%s", namespace),
				Status:    types.StatusError,
				Message:   fmt.Sprintf("获取命名空间 %s 的Deployment列表失败: %v", namespace, err),
				Resource:  namespace,
				Severity:  types.SeverityLow,
				Timestamp: time.Now(),
			})
			continue
		}

		totalDeployments += len(deployments.Items)

		// 检查每个Deployment
		for _, deployment := range deployments.Items {
			deploymentResults := c.checkDeployment(ctx, client, deployment)
			results = append(results, deploymentResults...)

			// 统计不健康的Deployment
			for _, result := range deploymentResults {
				if result.Status == types.StatusFail || result.Status == types.StatusWarning {
					unhealthyDeployments++
					break
				}
			}
		}
	}

	// 添加汇总信息
	results = append(results, types.CheckDetail{
		Category:  c.Category(),
		CheckName: c.Name(),
		CheckID:   "deployment_summary",
		Status: func() types.CheckStatus {
			if unhealthyDeployments == 0 {
				return types.StatusPass
			} else if unhealthyDeployments < totalDeployments/2 {
				return types.StatusWarning
			}
			return types.StatusFail
		}(),
		Message:      fmt.Sprintf("Deployment健康状态: 总数 %d, 有问题 %d", totalDeployments, unhealthyDeployments),
		Resource:     "cluster",
		ResourceType: "cluster",
		Severity:     types.SeverityLow,
		Evidence: map[string]interface{}{
			"total_deployments":     totalDeployments,
			"unhealthy_deployments": unhealthyDeployments,
			"namespaces":            len(namespaces.Items),
		},
		Timestamp: time.Now(),
	})

	duration := time.Since(startTime)
	logger.Infow("完成Deployment检查",
		"checker", c.Name(),
		"duration", duration,
		"total_deployments", totalDeployments,
		"unhealthy_deployments", unhealthyDeployments)

	return results, nil
}

func (c *DeploymentChecker) checkDeployment(ctx context.Context, client *k8s.Client, deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail

	// 1. 检查副本状态
	results = append(results, c.checkReplicaStatus(deployment)...)

	// 2. 检查更新策略
	results = append(results, c.checkUpdateStrategy(deployment)...)

	// 3. 检查滚动更新状态
	results = append(results, c.checkRollingUpdateStatus(deployment)...)

	// 4. 检查选择器匹配
	results = append(results, c.checkSelectorMatch(deployment)...)

	// 5. 检查资源限制
	results = append(results, c.checkDeploymentResources(deployment)...)

	// 6. 检查标签和注解
	results = append(results, c.checkLabelsAndAnnotations(deployment)...)

	return results
}

func (c *DeploymentChecker) checkReplicaStatus(deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail
	deploymentName := deployment.Name
	namespace := deployment.Namespace

	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}

	availableReplicas := deployment.Status.AvailableReplicas
	readyReplicas := deployment.Status.ReadyReplicas
	updatedReplicas := deployment.Status.UpdatedReplicas

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Deployment %s/%s 副本状态正常", namespace, deploymentName)

	// 检查副本数
	if desiredReplicas == 0 {
		status = types.StatusWarning
		severity = types.SeverityLow
		message = fmt.Sprintf("Deployment %s/%s 副本数为0", namespace, deploymentName)
	} else if availableReplicas < desiredReplicas {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Deployment %s/%s 可用副本不足: %d/%d",
			namespace, deploymentName, availableReplicas, desiredReplicas)
	} else if readyReplicas < desiredReplicas {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("Deployment %s/%s 就绪副本不足: %d/%d",
			namespace, deploymentName, readyReplicas, desiredReplicas)
	} else if updatedReplicas < desiredReplicas {
		status = types.StatusWarning
		severity = types.SeverityLow
		message = fmt.Sprintf("Deployment %s/%s 已更新副本不足: %d/%d",
			namespace, deploymentName, updatedReplicas, desiredReplicas)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "replica_status",
		Status:       status,
		Message:      message,
		Resource:     deploymentName,
		ResourceType: "Deployment",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"desired_replicas":   desiredReplicas,
			"available_replicas": availableReplicas,
			"ready_replicas":     readyReplicas,
			"updated_replicas":   updatedReplicas,
			"replicas":           deployment.Status.Replicas,
			"conditions":         deployment.Status.Conditions,
		},
		Suggestions: func() []string {
			if availableReplicas < desiredReplicas {
				return []string{
					"检查Pod创建失败的原因",
					"查看Deployment事件: kubectl describe deployment " + deploymentName + " -n " + namespace,
					"检查资源配额是否足够",
				}
			}
			if readyReplicas < desiredReplicas {
				return []string{
					"检查Pod的就绪探针配置",
					"查看Pod日志排查启动问题",
				}
			}
			if desiredReplicas == 0 {
				return []string{"确认是否需要将副本数设置为0，这可能影响服务可用性"}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *DeploymentChecker) checkUpdateStrategy(deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail
	deploymentName := deployment.Name
	namespace := deployment.Namespace

	strategy := deployment.Spec.Strategy
	maxUnavailable := "25%"
	maxSurge := "25%"

	if strategy.RollingUpdate != nil {
		if strategy.RollingUpdate.MaxUnavailable != nil {
			maxUnavailable = strategy.RollingUpdate.MaxUnavailable.String()
		}
		if strategy.RollingUpdate.MaxSurge != nil {
			maxSurge = strategy.RollingUpdate.MaxSurge.String()
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Deployment %s/%s 更新策略: %s",
		namespace, deploymentName, strategy.Type)

	// 检查更新策略类型
	if strategy.Type == appsv1.RecreateDeploymentStrategyType {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("Deployment %s/%s 使用Recreate策略，会导致服务中断",
			namespace, deploymentName)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "update_strategy",
		Status:       status,
		Message:      message,
		Resource:     deploymentName,
		ResourceType: "Deployment",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"strategy_type":   strategy.Type,
			"max_unavailable": maxUnavailable,
			"max_surge":       maxSurge,
		},
		Suggestions: func() []string {
			if strategy.Type == appsv1.RecreateDeploymentStrategyType {
				return []string{
					"考虑更改为RollingUpdate策略以实现零停机部署",
					"如果必须使用Recreate，确保有适当的多副本和负载均衡",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *DeploymentChecker) checkRollingUpdateStatus(deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail
	deploymentName := deployment.Name
	namespace := deployment.Namespace

	// 检查是否有进行中的滚动更新
	var progressingCondition *appsv1.DeploymentCondition
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			progressingCondition = &condition
			break
		}
	}

	if progressingCondition != nil && progressingCondition.Status == corev1.ConditionTrue {
		// 检查是否长时间处于更新中
		progressingDuration := time.Since(progressingCondition.LastUpdateTime.Time)

		if progressingDuration > 10*time.Minute {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   "rolling_update_stalled",
				Status:    types.StatusWarning,
				Message: fmt.Sprintf("Deployment %s/%s 滚动更新已进行 %v，可能已停滞",
					namespace, deploymentName, progressingDuration),
				Resource:     deploymentName,
				ResourceType: "Deployment",
				Namespace:    namespace,
				Severity:     types.SeverityMedium,
				Evidence: map[string]interface{}{
					"progressing_condition": progressingCondition,
					"duration":              progressingDuration.String(),
				},
				Suggestions: []string{
					"检查新Pod是否无法启动",
					"查看就绪探针配置",
					"检查资源限制是否足够",
					"考虑增加滚动更新超时时间",
				},
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

func (c *DeploymentChecker) checkSelectorMatch(deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail
	deploymentName := deployment.Name
	namespace := deployment.Namespace

	// 检查选择器是否有效
	_, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "selector_invalid",
			Status:    types.StatusFail,
			Message: fmt.Sprintf("Deployment %s/%s 选择器无效: %v",
				namespace, deploymentName, err),
			Resource:     deploymentName,
			ResourceType: "Deployment",
			Namespace:    namespace,
			Severity:     types.SeverityHigh,
			Timestamp:    time.Now(),
		})
		return results
	}

	// 注意：在实际实现中，需要检查Pod是否匹配选择器
	// 这里只是结构示例

	return results
}

func (c *DeploymentChecker) checkDeploymentResources(deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail
	deploymentName := deployment.Name
	namespace := deployment.Namespace

	// 检查Pod模板中的资源限制
	podSpec := deployment.Spec.Template.Spec

	for _, container := range podSpec.Containers {
		// 检查是否设置资源限制
		if container.Resources.Limits == nil || len(container.Resources.Limits) == 0 {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   "container_no_limits",
				Status:    types.StatusWarning,
				Message: fmt.Sprintf("Deployment %s/%s 容器 %s 未设置资源限制",
					namespace, deploymentName, container.Name),
				Resource:     deploymentName,
				ResourceType: "Deployment",
				Namespace:    namespace,
				Severity:     types.SeverityMedium,
				Evidence: map[string]interface{}{
					"container": container.Name,
				},
				Suggestions: []string{
					"为容器设置CPU和内存限制",
					"资源限制可以防止单个容器占用过多资源",
				},
				Timestamp: time.Now(),
			})
		}

		// 检查是否设置资源请求
		if container.Resources.Requests == nil || len(container.Resources.Requests) == 0 {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   "container_no_requests",
				Status:    types.StatusWarning,
				Message: fmt.Sprintf("Deployment %s/%s 容器 %s 未设置资源请求",
					namespace, deploymentName, container.Name),
				Resource:     deploymentName,
				ResourceType: "Deployment",
				Namespace:    namespace,
				Severity:     types.SeverityLow,
				Evidence: map[string]interface{}{
					"container": container.Name,
				},
				Suggestions: []string{
					"为容器设置CPU和内存请求",
					"资源请求影响Pod调度",
				},
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

func (c *DeploymentChecker) checkLabelsAndAnnotations(deployment appsv1.Deployment) []types.CheckDetail {
	var results []types.CheckDetail
	deploymentName := deployment.Name
	namespace := deployment.Namespace

	// 检查是否有版本标签
	labels := deployment.Spec.Template.Labels
	hasVersionLabel := false

	for key := range labels {
		if strings.Contains(strings.ToLower(key), "version") ||
			strings.Contains(strings.ToLower(key), "app.kubernetes.io/version") {
			hasVersionLabel = true
			break
		}
	}

	if !hasVersionLabel {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "missing_version_label",
			Status:       types.StatusWarning,
			Message:      fmt.Sprintf("Deployment %s/%s 缺少版本标签", namespace, deploymentName),
			Resource:     deploymentName,
			ResourceType: "Deployment",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Suggestions: []string{
				"添加版本标签以便跟踪和管理",
				"建议使用app.kubernetes.io/version标签",
			},
			Timestamp: time.Now(),
		})
	}

	// 检查是否有监控注解
	annotations := deployment.Spec.Template.Annotations
	hasMonitoringAnnotations := false

	for key := range annotations {
		if strings.Contains(key, "prometheus.io/") {
			hasMonitoringAnnotations = true
			break
		}
	}

	if !hasMonitoringAnnotations {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "missing_monitoring_annotations",
			Status:       types.StatusWarning,
			Message:      fmt.Sprintf("Deployment %s/%s 缺少监控注解", namespace, deploymentName),
			Resource:     deploymentName,
			ResourceType: "Deployment",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Suggestions: []string{
				"添加Prometheus监控注解",
				"例如: prometheus.io/scrape, prometheus.io/port",
			},
			Timestamp: time.Now(),
		})
	}

	return results
}
