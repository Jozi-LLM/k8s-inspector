// internal/checker/network/service_checker.go
package network

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

type ServiceChecker struct{}

func (c *ServiceChecker) Name() string {
	return "service_health"
}

func (c *ServiceChecker) Description() string {
	return "检查Service状态、端点和网络策略"
}

func (c *ServiceChecker) Category() string {
	return "network"
}

func (c *ServiceChecker) RequiredPermissions() []string {
	return []string{"get", "list", "watch"}
}

func (c *ServiceChecker) Execute(ctx context.Context, client *k8s.Client) ([]types.CheckDetail, error) {
	logger := utils.GetGlobalLogger()
	logger.Infow("开始执行Service检查", "checker", c.Name())

	startTime := time.Now()
	var results []types.CheckDetail

	// 获取所有命名空间的Service
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

	totalServices := 0
	problemServices := 0

	// 检查每个命名空间
	for _, ns := range namespaces.Items {
		namespace := ns.Name

		// 跳过系统命名空间
		if strings.HasPrefix(namespace, "kube-") {
			continue
		}

		services, err := client.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   fmt.Sprintf("service_list_%s", namespace),
				Status:    types.StatusError,
				Message:   fmt.Sprintf("获取命名空间 %s 的Service列表失败: %v", namespace, err),
				Resource:  namespace,
				Severity:  types.SeverityLow,
				Timestamp: time.Now(),
			})
			continue
		}

		totalServices += len(services.Items)

		// 检查每个Service
		for _, service := range services.Items {
			serviceResults := c.checkService(ctx, client, service)
			results = append(results, serviceResults...)

			// 统计有问题的Service
			for _, result := range serviceResults {
				if result.Status == types.StatusFail || result.Status == types.StatusWarning {
					problemServices++
					break
				}
			}
		}
	}

	// 添加汇总信息
	results = append(results, types.CheckDetail{
		Category:  c.Category(),
		CheckName: c.Name(),
		CheckID:   "service_summary",
		Status: func() types.CheckStatus {
			if problemServices == 0 {
				return types.StatusPass
			} else if problemServices < totalServices/2 {
				return types.StatusWarning
			}
			return types.StatusFail
		}(),
		Message:      fmt.Sprintf("Service健康状态: 总数 %d, 有问题 %d", totalServices, problemServices),
		Resource:     "cluster",
		ResourceType: "cluster",
		Severity:     types.SeverityLow,
		Evidence: map[string]interface{}{
			"total_services":   totalServices,
			"problem_services": problemServices,
			"namespaces":       len(namespaces.Items),
		},
		Timestamp: time.Now(),
	})

	duration := time.Since(startTime)
	logger.Infow("完成Service检查",
		"checker", c.Name(),
		"duration", duration,
		"total_services", totalServices,
		"problem_services", problemServices)

	return results, nil
}

func (c *ServiceChecker) checkService(ctx context.Context, client *k8s.Client, service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail

	// 1. 检查Service类型
	results = append(results, c.checkServiceType(service)...)

	// 2. 检查端口配置
	results = append(results, c.checkPorts(service)...)

	// 3. 检查选择器和端点
	results = append(results, c.checkSelectorAndEndpoints(ctx, client, service)...)

	// 4. 检查负载均衡器状态
	results = append(results, c.checkLoadBalancer(ctx, client, service)...)

	// 5. 检查外部流量策略
	results = append(results, c.checkExternalTrafficPolicy(service)...)

	// 6. 检查会话亲和性
	results = append(results, c.checkSessionAffinity(service)...)

	return results
}

func (c *ServiceChecker) checkServiceType(service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail
	serviceName := service.Name
	namespace := service.Namespace

	serviceType := service.Spec.Type
	clusterIP := service.Spec.ClusterIP

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Service %s/%s 类型: %s", namespace, serviceName, serviceType)

	// 检查ClusterIP
	if serviceType == corev1.ServiceTypeClusterIP {
		if clusterIP == "None" {
			message = fmt.Sprintf("Service %s/%s 是Headless Service (ClusterIP: None)",
				namespace, serviceName)
		} else if clusterIP == "" {
			status = types.StatusWarning
			severity = types.SeverityLow
			message = fmt.Sprintf("Service %s/%s ClusterIP为空", namespace, serviceName)
		}
	}

	// 检查NodePort
	if serviceType == corev1.ServiceTypeNodePort {
		// NodePort服务应该有分配的端口
		hasNodePort := false
		for _, port := range service.Spec.Ports {
			if port.NodePort > 0 {
				hasNodePort = true
				break
			}
		}

		if !hasNodePort {
			status = types.StatusWarning
			severity = types.SeverityMedium
			message = fmt.Sprintf("Service %s/%s 类型为NodePort但未分配节点端口",
				namespace, serviceName)
		}
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "service_type",
		Status:       status,
		Message:      message,
		Resource:     serviceName,
		ResourceType: "Service",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"type":       serviceType,
			"cluster_ip": clusterIP,
		},
		Timestamp: time.Now(),
	})

	return results
}

func (c *ServiceChecker) checkPorts(service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail
	serviceName := service.Name
	namespace := service.Namespace

	ports := service.Spec.Ports

	if len(ports) == 0 {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "no_ports",
			Status:       types.StatusFail,
			Message:      fmt.Sprintf("Service %s/%s 未定义任何端口", namespace, serviceName),
			Resource:     serviceName,
			ResourceType: "Service",
			Namespace:    namespace,
			Severity:     types.SeverityHigh,
			Suggestions: []string{
				"为Service添加端口定义",
				"端口是Service路由流量的必要条件",
			},
			Timestamp: time.Now(),
		})
		return results
	}

	// 检查端口配置
	for _, port := range ports {
		// 检查端口名称
		if port.Name == "" {
			results = append(results, types.CheckDetail{
				Category:  c.Category(),
				CheckName: c.Name(),
				CheckID:   "port_name_missing",
				Status:    types.StatusWarning,
				Message: fmt.Sprintf("Service %s/%s 端口 %d 缺少名称",
					namespace, serviceName, port.Port),
				Resource:     serviceName,
				ResourceType: "Service",
				Namespace:    namespace,
				Severity:     types.SeverityLow,
				Evidence: map[string]interface{}{
					"port":        port.Port,
					"target_port": port.TargetPort.String(),
				},
				Suggestions: []string{
					"为端口设置一个有意义的名称",
					"端口名称在Istio等Service Mesh中有特殊用途",
				},
				Timestamp: time.Now(),
			})
		}

		// 检查端口协议
		if port.Protocol == "" {
			port.Protocol = corev1.ProtocolTCP
		}

		// 检查端口冲突（简单检查）
		if port.Port == 0 {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "port_zero",
				Status:       types.StatusFail,
				Message:      fmt.Sprintf("Service %s/%s 有端口号为0", namespace, serviceName),
				Resource:     serviceName,
				ResourceType: "Service",
				Namespace:    namespace,
				Severity:     types.SeverityHigh,
				Timestamp:    time.Now(),
			})
		}
	}

	// 添加端口摘要
	portSummary := make([]map[string]interface{}, len(ports))
	for i, port := range ports {
		portSummary[i] = map[string]interface{}{
			"name":        port.Name,
			"port":        port.Port,
			"target_port": port.TargetPort.String(),
			"node_port":   port.NodePort,
			"protocol":    string(port.Protocol),
		}
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "port_summary",
		Status:       types.StatusPass,
		Message:      fmt.Sprintf("Service %s/%s 端口配置", namespace, serviceName),
		Resource:     serviceName,
		ResourceType: "Service",
		Namespace:    namespace,
		Severity:     types.SeverityLow,
		Evidence: map[string]interface{}{
			"ports":      portSummary,
			"port_count": len(ports),
		},
		Timestamp: time.Now(),
	})

	return results
}

func (c *ServiceChecker) checkSelectorAndEndpoints(ctx context.Context, client *k8s.Client, service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail
	serviceName := service.Name
	namespace := service.Namespace

	selector := service.Spec.Selector

	// 检查是否有选择器（无选择器表示外部服务或手动管理的端点）
	if len(selector) == 0 {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "no_selector",
			Status:       types.StatusWarning,
			Message:      fmt.Sprintf("Service %s/%s 没有选择器", namespace, serviceName),
			Resource:     serviceName,
			ResourceType: "Service",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Evidence: map[string]interface{}{
				"selector": selector,
			},
			Suggestions: []string{
				"如果这是外部服务，请确认端点已正确配置",
				"如果是集群内服务，请添加选择器以自动管理端点",
			},
			Timestamp: time.Now(),
		})

		// 检查是否有手动配置的端点
		endpoints, err := client.Clientset.CoreV1().Endpoints(namespace).Get(ctx, serviceName, metav1.GetOptions{})
		if err != nil {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "endpoints_not_found",
				Status:       types.StatusFail,
				Message:      fmt.Sprintf("Service %s/%s 无选择器且未找到端点", namespace, serviceName),
				Resource:     serviceName,
				ResourceType: "Service",
				Namespace:    namespace,
				Severity:     types.SeverityHigh,
				Timestamp:    time.Now(),
			})
		} else if len(endpoints.Subsets) == 0 {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "no_endpoints",
				Status:       types.StatusFail,
				Message:      fmt.Sprintf("Service %s/%s 没有可用的端点", namespace, serviceName),
				Resource:     serviceName,
				ResourceType: "Service",
				Namespace:    namespace,
				Severity:     types.SeverityHigh,
				Timestamp:    time.Now(),
			})
		}

		return results
	}

	// 有选择器，检查是否有匹配的Pod
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: selector,
		}),
	})

	if err != nil {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "pod_list_error",
			Status:       types.StatusError,
			Message:      fmt.Sprintf("检查Service %s/%s 匹配Pod失败: %v", namespace, serviceName, err),
			Resource:     serviceName,
			ResourceType: "Service",
			Namespace:    namespace,
			Severity:     types.SeverityMedium,
			Timestamp:    time.Now(),
		})
		return results
	}

	// 检查匹配的Pod数量
	readyPods := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					readyPods++
					break
				}
			}
		}
	}

	totalPods := len(pods.Items)

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("Service %s/%s 有 %d 个就绪端点", namespace, serviceName, readyPods)

	if totalPods == 0 {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Service %s/%s 没有匹配的Pod", namespace, serviceName)
	} else if readyPods == 0 {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("Service %s/%s 有 %d 个Pod但没有就绪的", namespace, serviceName, totalPods)
	} else if readyPods < totalPods {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("Service %s/%s 只有 %d/%d 个Pod就绪",
			namespace, serviceName, readyPods, totalPods)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "endpoint_availability",
		Status:       status,
		Message:      message,
		Resource:     serviceName,
		ResourceType: "Service",
		Namespace:    namespace,
		Severity:     severity,
		Evidence: map[string]interface{}{
			"selector":       selector,
			"total_pods":     totalPods,
			"ready_pods":     readyPods,
			"not_ready_pods": totalPods - readyPods,
		},
		Suggestions: func() []string {
			if totalPods == 0 {
				return []string{
					"检查Pod标签是否与Service选择器匹配",
					"确认Pod已部署到正确的命名空间",
				}
			}
			if readyPods == 0 {
				return []string{
					"检查Pod的就绪探针配置",
					"查看Pod日志排查启动问题",
				}
			}
			if readyPods < totalPods {
				return []string{
					"检查未就绪Pod的状态和事件",
					"确保所有Pod都能通过就绪探针",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *ServiceChecker) checkLoadBalancer(ctx context.Context, client *k8s.Client, service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail
	serviceName := service.Name
	namespace := service.Namespace

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return results
	}

	// 检查LoadBalancer状态
	ingress := service.Status.LoadBalancer.Ingress

	if len(ingress) == 0 {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "loadbalancer_pending",
			Status:       types.StatusWarning,
			Message:      fmt.Sprintf("LoadBalancer服务 %s/%s 等待分配外部IP", namespace, serviceName),
			Resource:     serviceName,
			ResourceType: "Service",
			Namespace:    namespace,
			Severity:     types.SeverityMedium,
			Evidence: map[string]interface{}{
				"type":    service.Spec.Type,
				"ingress": ingress,
			},
			Suggestions: []string{
				"检查云提供商的负载均衡器配置",
				"验证网络插件是否支持LoadBalancer",
				"如果是本地集群，考虑使用MetalLB",
			},
			Timestamp: time.Now(),
		})
	} else {
		// 检查LoadBalancer IP或主机名
		var addresses []string
		for _, ing := range ingress {
			if ing.IP != "" {
				addresses = append(addresses, ing.IP)
			} else if ing.Hostname != "" {
				addresses = append(addresses, ing.Hostname)
			}
		}

		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "loadbalancer_ready",
			Status:    types.StatusPass,
			Message: fmt.Sprintf("LoadBalancer服务 %s/%s 已就绪: %s",
				namespace, serviceName, strings.Join(addresses, ", ")),
			Resource:     serviceName,
			ResourceType: "Service",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Evidence: map[string]interface{}{
				"ingress":   ingress,
				"addresses": addresses,
			},
			Timestamp: time.Now(),
		})
	}

	return results
}

func (c *ServiceChecker) checkExternalTrafficPolicy(service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail
	serviceName := service.Name
	namespace := service.Namespace

	policy := service.Spec.ExternalTrafficPolicy
	if policy == "" {
		policy = corev1.ServiceExternalTrafficPolicyTypeCluster
	}

	// 对于NodePort和LoadBalancer服务，检查外部流量策略
	if service.Spec.Type == corev1.ServiceTypeNodePort || service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if policy == corev1.ServiceExternalTrafficPolicyTypeLocal {
			results = append(results, types.CheckDetail{
				Category:     c.Category(),
				CheckName:    c.Name(),
				CheckID:      "external_traffic_policy_local",
				Status:       types.StatusPass,
				Message:      fmt.Sprintf("Service %s/%s 使用Local外部流量策略", namespace, serviceName),
				Resource:     serviceName,
				ResourceType: "Service",
				Namespace:    namespace,
				Severity:     types.SeverityLow,
				Evidence: map[string]interface{}{
					"external_traffic_policy": policy,
				},
				Suggestions: []string{
					"Local策略保留客户端IP但可能导致负载不均",
					"确保所有节点都有后端Pod以避免流量丢失",
				},
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

func (c *ServiceChecker) checkSessionAffinity(service corev1.Service) []types.CheckDetail {
	var results []types.CheckDetail
	serviceName := service.Name
	namespace := service.Namespace

	affinity := service.Spec.SessionAffinity
	if affinity == "" {
		affinity = corev1.ServiceAffinityNone
	}

	if affinity == corev1.ServiceAffinityClientIP {
		timeout := service.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds
		if timeout == nil || *timeout == 0 {
			// 使用默认值10800秒（3小时）
			defaultTimeout := int32(10800)
			timeout = &defaultTimeout
		}

		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "session_affinity_enabled",
			Status:    types.StatusPass,
			Message: fmt.Sprintf("Service %s/%s 启用了会话亲和性(超时: %d秒)",
				namespace, serviceName, *timeout),
			Resource:     serviceName,
			ResourceType: "Service",
			Namespace:    namespace,
			Severity:     types.SeverityLow,
			Evidence: map[string]interface{}{
				"session_affinity": affinity,
				"timeout_seconds":  timeout,
			},
			Suggestions: []string{
				"会话亲和性确保同一客户端的请求发送到同一Pod",
				"适用于需要保持会话状态的应用",
			},
			Timestamp: time.Now(),
		})
	}

	return results
}
