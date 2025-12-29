// internal/checker/workload/pod_health.go
package workload

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

type PodHealthChecker struct{}

func (c *PodHealthChecker) Name() string {
	return "pod_health"
}

func (c *PodHealthChecker) Description() string {
	return "检查Pod状态、重启次数、资源限制和镜像拉取"
}

func (c *PodHealthChecker) Category() string {
	return "workload"
}

func (c *PodHealthChecker) RequiredPermissions() []string {
	return []string{"get", "list", "watch"}
}

func (c *PodHealthChecker) Execute(ctx context.Context, client *k8s.Client) ([]types.CheckDetail, error) {
	logger := utils.GetGlobalLogger()
	logger.Infow("开始执行Pod健康检查", "checker", c.Name())

	startTime := time.Now()
	var results []types.CheckDetail

	// 获取所有命名空间的Pod
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

	totalPods := 0
	problemPods := 0

	// 检查每个命名空间
	for _, ns := range namespaces.Items {
		namespace := ns.Name

		// 跳过系统命名空间
		if strings.HasPrefix(namespace, "kube-") {
			continue
		}

		pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   fmt.Sprintf("pod_list_%s", namespace),
				Status:    types.StatusError,
				Message:   fmt.Sprintf("获取命名空间 %s 的Pod列表失败: %v", namespace, err),
				Resource:  namespace,
				Severity:  types.SeverityLow,
				Timestamp: time.Now(),
			})
			continue
		}

		totalPods += len(pods.Items)

		// 检查每个Pod
		for _, pod := range pods.Items {
			podResults := c.checkPod(ctx, client, pod)
			results = append(results, podResults...)

			// 统计有问题的Pod
			for _, result := range podResults {
				if result.Status == types.StatusFail || result.Status == types.StatusWarning {
					problemPods++
					break
				}
			}
		}
	}

	// 添加汇总信息
	results = append(results, types.CheckDetail{
		Category:  c.Category(),
		CheckName: c.Name(),
		CheckID:   "pod_summary",
		Status: func() types.CheckStatus {
			if problemPods == 0 {
				return types.StatusPass
			} else if problemPods < totalPods/2 {
				return types.StatusWarning
			}
			return types.StatusFail
		}(),
		Message:      fmt.Sprintf("Pod健康状态: 总数 %d, 有问题 %d", totalPods, problemPods),
		Resource:     "cluster",
		ResourceType: "cluster",
		Severity:     types.SeverityLow,
		Evidence: map[string]interface{}{
			"total_pods":   totalPods,
			"problem_pods": problemPods,
			"namespaces":   len(namespaces.Items),
		},
		Timestamp: time.Now(),
	})

	duration := time.Since(startTime)
	logger.Infow("完成Pod健康检查",
		"checker", c.Name(),
		"duration", duration,
		"total_pods", totalPods,
		"problem_pods", problemPods)

	return results, nil
}

func (c *PodHealthChecker) checkPod(ctx context.Context, client *k8s.Client, pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail

	// 1. 检查Pod状态
	results = append(results, c.checkPodStatus(pod)...)

	// 2. 检查容器状态
	results = append(results, c.checkContainerStatus(pod)...)

	// 3. 检查重启次数
	results = append(results, c.checkRestartCount(pod)...)

	// 4. 检查资源限制
	results = append(results, c.checkResourceLimits(pod)...)

	// 5. 检查镜像拉取策略
	results = append(results, c.checkImagePullPolicy(pod)...)

	// 6. 检查亲和性和反亲和性
	results = append(results, c.checkAffinity(pod)...)

	// 7. 检查探针配置
	results = append(results, c.checkProbes(pod)...)

	// 8. 检查安全上下文
	results = append(results, c.checkSecurityContext(pod)...)

	return results
}

func (c *PodHealthChecker) checkPodStatus(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Pod %s/%s 状态正常", namespace, podName)

	// 检查Pod阶段
	switch pod.Status.Phase {
	case corev1.PodPending:
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("Pod %s/%s 处于Pending状态", namespace, podName)

	case corev1.PodFailed:
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Pod %s/%s 处于Failed状态", namespace, podName)

	case corev1.PodUnknown:
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Pod %s/%s 状态未知", namespace, podName)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "pod_phase",
		Status:       status,
		Message:      message,
		Resource:     podName,
		ResourceType: "Pod",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"phase":   pod.Status.Phase,
			"reason":  pod.Status.Reason,
			"message": pod.Status.Message,
			"qos":     pod.Status.QOSClass,
		},
		Suggestions: func() []string {
			switch pod.Status.Phase {
			case corev1.PodPending:
				return []string{
					"检查Pod事件: kubectl describe pod " + podName + " -n " + namespace,
					"检查节点资源是否充足",
					"检查PVC绑定状态",
					"检查镜像拉取权限",
				}
			case corev1.PodFailed:
				return []string{
					"查看Pod日志: kubectl logs " + podName + " -n " + namespace,
					"检查容器退出代码",
					"查看容器启动命令是否正确",
				}
			case corev1.PodUnknown:
				return []string{
					"检查节点状态",
					"查看kubelet日志",
					"检查网络连接",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *PodHealthChecker) checkContainerStatus(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	allRunning := true
	hasWaiting := false
	hasTerminated := false
	hasCrashLoopBackOff := false

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Running == nil {
			allRunning = false
		}

		if containerStatus.State.Waiting != nil {
			hasWaiting = true
			if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
				hasCrashLoopBackOff = true
			}
		}

		if containerStatus.State.Terminated != nil {
			hasTerminated = true
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Pod %s/%s 所有容器运行正常", namespace, podName)

	if hasCrashLoopBackOff {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Pod %s/%s 有容器处于CrashLoopBackOff状态", namespace, podName)
	} else if hasTerminated {
		status = types.StatusFail
		severity = types.SeverityMedium
		message = fmt.Sprintf("Pod %s/%s 有容器已终止", namespace, podName)
	} else if hasWaiting {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("Pod %s/%s 有容器处于等待状态", namespace, podName)
	} else if !allRunning {
		status = types.StatusWarning
		severity = types.SeverityLow
		message = fmt.Sprintf("Pod %s/%s 容器状态异常", namespace, podName)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "container_status",
		Status:       status,
		Message:      message,
		Resource:     podName,
		ResourceType: "Pod",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"container_statuses": pod.Status.ContainerStatuses,
			"total_containers":   len(pod.Spec.Containers),
			"running_containers": allRunning,
		},
		Suggestions: func() []string {
			if hasCrashLoopBackOff {
				return []string{
					"检查应用启动脚本和配置",
					"查看容器日志排查崩溃原因",
					"检查依赖的服务是否可用",
					"考虑增加initialDelaySeconds",
				}
			}
			if hasTerminated {
				return []string{"查看容器退出代码和原因"}
			}
			if hasWaiting {
				return []string{"检查容器等待原因: ImagePullBackOff, ContainerCreating等"}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *PodHealthChecker) checkRestartCount(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	maxRestarts := 0
	var restartingContainers []string

	for _, containerStatus := range pod.Status.ContainerStatuses {
		restarts := int(containerStatus.RestartCount)
		if restarts > maxRestarts {
			maxRestarts = restarts
		}
		if restarts > 0 {
			restartingContainers = append(restartingContainers,
				fmt.Sprintf("%s(%d)", containerStatus.Name, restarts))
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Pod %s/%s 重启次数正常", namespace, podName)

	if maxRestarts > 50 {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Pod %s/%s 重启次数过多: %d次",
			namespace, podName, maxRestarts)
	} else if maxRestarts > 10 {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("Pod %s/%s 重启次数较多: %d次",
			namespace, podName, maxRestarts)
	} else if maxRestarts > 0 {
		status = types.StatusWarning
		severity = types.SeverityLow
		message = fmt.Sprintf("Pod %s/%s 有容器重启过: %s",
			namespace, podName, strings.Join(restartingContainers, ", "))
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "restart_count",
		Status:       status,
		Message:      message,
		Resource:     podName,
		ResourceType: "Pod",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"max_restarts":          maxRestarts,
			"restarting_containers": restartingContainers,
			"age":                   time.Since(pod.CreationTimestamp.Time).String(),
		},
		Suggestions: func() []string {
			if maxRestarts > 0 {
				return []string{
					"检查容器日志查找重启原因",
					"检查内存限制是否过小",
					"检查应用是否有内存泄漏",
					"检查健康检查探针配置",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *PodHealthChecker) checkResourceLimits(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	hasLimits := false
	hasRequests := false
	var containersWithoutLimits []string
	var containersWithoutRequests []string

	for _, container := range pod.Spec.Containers {
		if container.Resources.Limits == nil || len(container.Resources.Limits) == 0 {
			containersWithoutLimits = append(containersWithoutLimits, container.Name)
		} else {
			hasLimits = true
		}

		if container.Resources.Requests == nil || len(container.Resources.Requests) == 0 {
			containersWithoutRequests = append(containersWithoutRequests, container.Name)
		} else {
			hasRequests = true
		}
	}

	// 检查是否设置资源限制
	if len(containersWithoutLimits) > 0 {
		status := types.StatusWarning
		severity := types.SeverityMedium

		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "resource_limits_missing",
			Status:    status,
			Message: fmt.Sprintf("Pod %s/%s 的容器未设置资源限制: %s",
				namespace, podName, strings.Join(containersWithoutLimits, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     severity,
			Evidence: map[string]interface{}{
				"containers_without_limits": containersWithoutLimits,
			},
			Suggestions: []string{
				"为所有容器设置CPU和内存限制",
				"参考应用实际使用情况设置合理的限制",
				"使用Vertical Pod Autoscaler自动设置资源",
			},
			Timestamp: time.Now(),
		})
	}

	// 检查是否设置资源请求
	if len(containersWithoutRequests) > 0 {
		status := types.StatusWarning
		severity := types.SeverityLow

		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "resource_requests_missing",
			Status:    status,
			Message: fmt.Sprintf("Pod %s/%s 的容器未设置资源请求: %s",
				namespace, podName, strings.Join(containersWithoutRequests, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     severity,
			Evidence: map[string]interface{}{
				"containers_without_requests": containersWithoutRequests,
			},
			Suggestions: []string{
				"为所有容器设置CPU和内存请求",
				"资源请求应该反映应用的最小资源需求",
			},
			Timestamp: time.Now(),
		})
	}

	// 如果都设置了，检查请求和限制的比例
	if hasLimits && hasRequests {
		for _, container := range pod.Spec.Containers {
			// 检查CPU请求和限制比例
			if cpuLimit, hasCPULimit := container.Resources.Limits[corev1.ResourceCPU]; hasCPULimit {
				if cpuRequest, hasCPURequest := container.Resources.Requests[corev1.ResourceCPU]; hasCPURequest {
					cpuLimitQty, _ := cpuLimit.AsInt64()
					cpuRequestQty, _ := cpuRequest.AsInt64()

					if cpuRequestQty > 0 {
						cpuRatio := float64(cpuLimitQty) / float64(cpuRequestQty)
						if cpuRatio > 8 {
							results = append(results, types.CheckDetail{
								Category:  c.Category(),
								CheckName: c.Name(),
								CheckID:   "cpu_limit_ratio_high",
								Status:    types.StatusWarning,
								Message: fmt.Sprintf("Pod %s/%s 容器 %s CPU限制与请求比例过高: %.1f倍",
									namespace, podName, container.Name, cpuRatio),
								Resource:     podName,
								ResourceType: "Pod",
								Namespace:    namespace,
								Severity:     types.SeverityLow,
								Evidence: map[string]interface{}{
									"container":           container.Name,
									"cpu_limit":           cpuLimit.String(),
									"cpu_request":         cpuRequest.String(),
									"limit_request_ratio": cpuRatio,
								},
								Suggestions: []string{
									"考虑调整CPU请求和限制，使其比例更合理",
									"过高的比例可能导致资源争用",
								},
								Timestamp: time.Now(),
							})
						}
					}
				}
			}

			// 检查内存请求和限制比例
			if memLimit, hasMemLimit := container.Resources.Limits[corev1.ResourceMemory]; hasMemLimit {
				if memRequest, hasMemRequest := container.Resources.Requests[corev1.ResourceMemory]; hasMemRequest {
					memLimitQty := memLimit.Value()
					memRequestQty := memRequest.Value()

					if memRequestQty > 0 {
						memRatio := float64(memLimitQty) / float64(memRequestQty)
						if memRatio > 2 {
							results = append(results, types.CheckDetail{
								Category:  c.Category(),
								CheckName: c.Name(),
								CheckID:   "memory_limit_ratio_high",
								Status:    types.StatusWarning,
								Message: fmt.Sprintf("Pod %s/%s 容器 %s 内存限制与请求比例过高: %.1f倍",
									namespace, podName, container.Name, memRatio),
								Resource:     podName,
								ResourceType: "Pod",
								Namespace:    namespace,
								Severity:     types.SeverityMedium,
								Evidence: map[string]interface{}{
									"container":           container.Name,
									"memory_limit":        memLimit.String(),
									"memory_request":      memRequest.String(),
									"limit_request_ratio": memRatio,
								},
								Suggestions: []string{
									"内存限制不应超过请求的2倍，否则可能导致OOM Kill",
									"根据应用实际内存使用模式调整比例",
								},
								Timestamp: time.Now(),
							})
						}
					}
				}
			}
		}
	}

	return results
}

func (c *PodHealthChecker) checkImagePullPolicy(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	var containersWithLatestTag []string
	var containersWithoutPullPolicy []string

	for _, container := range pod.Spec.Containers {
		// 检查是否使用latest标签
		imageParts := strings.Split(container.Image, ":")
		if len(imageParts) == 1 || imageParts[1] == "latest" {
			containersWithLatestTag = append(containersWithLatestTag, container.Name)
		}

		// 检查是否设置镜像拉取策略
		if container.ImagePullPolicy == "" {
			containersWithoutPullPolicy = append(containersWithoutPullPolicy, container.Name)
		}
	}

	// 检查是否使用latest标签
	if len(containersWithLatestTag) > 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "image_latest_tag",
			Status:    types.StatusWarning,
			Message: fmt.Sprintf("Pod %s/%s 的容器使用latest标签: %s",
				namespace, podName, strings.Join(containersWithLatestTag, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     types.SeverityMedium,
			Evidence: map[string]interface{}{
				"containers_with_latest": containersWithLatestTag,
			},
			Suggestions: []string{
				"避免使用latest标签，改用具体版本号",
				"使用语义化版本控制",
				"考虑使用镜像摘要(sha256)",
			},
			Timestamp: time.Now(),
		})
	}

	// 检查镜像拉取策略
	if len(containersWithoutPullPolicy) > 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "image_pull_policy_missing",
			Status:    types.StatusWarning,
			Message: fmt.Sprintf("Pod %s/%s 的容器未设置镜像拉取策略: %s",
				namespace, podName, strings.Join(containersWithoutPullPolicy, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Evidence: map[string]interface{}{
				"containers_without_policy": containersWithoutPullPolicy,
			},
			Suggestions: []string{
				"显式设置ImagePullPolicy",
				"生产环境建议使用IfNotPresent或Always",
			},
			Timestamp: time.Now(),
		})
	}

	return results
}

func (c *PodHealthChecker) checkAffinity(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	// 检查Pod亲和性配置
	if pod.Spec.Affinity != nil {
		// 检查节点亲和性
		if pod.Spec.Affinity.NodeAffinity != nil {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "node_affinity_set",
				Status:       types.StatusPass,
				Message:      fmt.Sprintf("Pod %s/%s 设置了节点亲和性", namespace, podName),
				Resource:     podName,
				ResourceType: "Pod",
				Namespace:    namespace,
				Severity:     types.SeverityLow,
				Evidence: map[string]interface{}{
					"node_affinity": pod.Spec.Affinity.NodeAffinity,
				},
				Timestamp: time.Now(),
			})
		}

		// 检查Pod亲和性
		if pod.Spec.Affinity.PodAffinity != nil {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "pod_affinity_set",
				Status:       types.StatusPass,
				Message:      fmt.Sprintf("Pod %s/%s 设置了Pod亲和性", namespace, podName),
				Resource:     podName,
				ResourceType: "Pod",
				Namespace:    namespace,
				Severity:     types.SeverityLow,
				Evidence: map[string]interface{}{
					"pod_affinity": pod.Spec.Affinity.PodAffinity,
				},
				Timestamp: time.Now(),
			})
		}

		// 检查Pod反亲和性
		if pod.Spec.Affinity.PodAntiAffinity != nil {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "pod_anti_affinity_set",
				Status:       types.StatusPass,
				Message:      fmt.Sprintf("Pod %s/%s 设置了Pod反亲和性", namespace, podName),
				Resource:     podName,
				ResourceType: "Pod",
				Namespace:    namespace,
				Severity:     types.SeverityLow,
				Evidence: map[string]interface{}{
					"pod_anti_affinity": pod.Spec.Affinity.PodAntiAffinity,
				},
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

func (c *PodHealthChecker) checkProbes(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	var containersWithoutReadiness []string
	var containersWithoutLiveness []string

	for _, container := range pod.Spec.Containers {
		if container.ReadinessProbe == nil {
			containersWithoutReadiness = append(containersWithoutReadiness, container.Name)
		}
		if container.LivenessProbe == nil {
			containersWithoutLiveness = append(containersWithoutLiveness, container.Name)
		}
		if container.StartupProbe == nil && container.ReadinessProbe != nil && container.LivenessProbe != nil {
			// 如果应用启动较慢，建议设置StartupProbe
			// 这里只是检查，不强制要求
		}
	}

	// 检查就绪探针
	if len(containersWithoutReadiness) > 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "readiness_probe_missing",
			Status:    types.StatusWarning,
			Message: fmt.Sprintf("Pod %s/%s 的容器未设置就绪探针: %s",
				namespace, podName, strings.Join(containersWithoutReadiness, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     types.SeverityMedium,
			Evidence: map[string]interface{}{
				"containers_without_readiness": containersWithoutReadiness,
			},
			Suggestions: []string{
				"为所有容器设置就绪探针",
				"就绪探针确保流量只转发到完全准备好的Pod",
			},
			Timestamp: time.Now(),
		})
	}

	// 检查存活探针
	if len(containersWithoutLiveness) > 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "liveness_probe_missing",
			Status:    types.StatusWarning,
			Message: fmt.Sprintf("Pod %s/%s 的容器未设置存活探针: %s",
				namespace, podName, strings.Join(containersWithoutLiveness, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Evidence: map[string]interface{}{
				"containers_without_liveness": containersWithoutLiveness,
			},
			Suggestions: []string{
				"为关键容器设置存活探针",
				"存活探针可以自动重启失败的容器",
			},
			Timestamp: time.Now(),
		})
	}

	return results
}

func (c *PodHealthChecker) checkSecurityContext(pod corev1.Pod) []types.CheckDetail {
	var results []types.CheckDetail
	podName := pod.Name
	namespace := pod.Namespace

	// 检查特权容器
	var privilegedContainers []string
	for _, container := range pod.Spec.Containers {
		if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
			privilegedContainers = append(privilegedContainers, container.Name)
		}
	}

	if len(privilegedContainers) > 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "privileged_container",
			Status:    types.StatusFail,
			Message: fmt.Sprintf("Pod %s/%s 有特权容器: %s",
				namespace, podName, strings.Join(privilegedContainers, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     types.SeverityHigh,
			Evidence: map[string]interface{}{
				"privileged_containers": privilegedContainers,
			},
			Suggestions: []string{
				"避免使用特权容器，除非绝对必要",
				"考虑使用更细粒度的Linux Capabilities",
				"使用安全上下文约束(SCC)或Pod安全策略(PSP)",
			},
			Timestamp: time.Now(),
		})
	}

	// 检查是否以root用户运行
	var rootContainers []string
	for _, container := range pod.Spec.Containers {
		if container.SecurityContext == nil ||
			(container.SecurityContext.RunAsUser == nil || *container.SecurityContext.RunAsUser == 0) {
			rootContainers = append(rootContainers, container.Name)
		}
	}

	if len(rootContainers) > 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "run_as_root",
			Status:    types.StatusWarning,
			Message: fmt.Sprintf("Pod %s/%s 的容器以root用户运行: %s",
				namespace, podName, strings.Join(rootContainers, ", ")),
			Resource:     podName,
			ResourceType: "Pod",
			Namespace:    namespace,
			Severity:     types.SeverityMedium,
			Evidence: map[string]interface{}{
				"root_containers": rootContainers,
			},
			Suggestions: []string{
				"避免以root用户运行容器",
				"创建非root用户运行应用",
				"在Dockerfile中使用USER指令",
			},
			Timestamp: time.Now(),
		})
	}

	return results
}

