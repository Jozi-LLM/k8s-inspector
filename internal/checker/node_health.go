// 节点健康检查器

package checker

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

type NodeHealthChecker struct {
}

func (c *NodeHealthChecker) Name() string {
	return "NodeHealthChecker"
}

func (c *NodeHealthChecker) Description() string {
	return "检查节点健康状态, 资源压力 内核参数"
}

func (c *NodeHealthChecker) Category() string {
	return "node"
}

func (c *NodeHealthChecker) RequiredPermissions() []string {
	return []string{"get", "list", "watch"}
}

func (c *NodeHealthChecker) Execute(ctx context.Context, client *k8s.Client) ([]types.CheckDetail, error) {
	logger := utils.GetGlobalLogger()
	logger.Infow("开始执行节点健康检查", "checker", c.Name())

	startTime := time.Now()
	var results []types.CheckDetail

	// 获取所有节点
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   c.Name(),
			Status:    types.StatusError,
			Message:   "获取节点列表失败",
			Severity:  types.SeverityCritical,
			Timestamp: time.Now(),
		})
		return results, err
	}
	if len(nodes.Items) == 0 {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   c.Name(),
			Status:    types.StatusError,
			Message:   "节点列表为空",
			Severity:  types.SeverityCritical,
			Timestamp: time.Now(),
		})
		return results, nil
	}
	logger.Debugw("开始检查节点", "node_count", len(nodes.Items))

	// 检查每个节点
	for _, node := range nodes.Items {
		nodeResults := c.checkNode(ctx, client, node)
		results = append(results, nodeResults...)
	}

	duration := time.Since(startTime)
	logger.Info("完成节点健康检查",
		"checker", c.Name(),
		"duration", duration,
		"nodes", len(results),
		"results", results)
	return results, nil
}

func (c *NodeHealthChecker) checkNode(ctx context.Context, client *k8s.Client, node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail

	// 1. 检查节点Readay状态
	results = append(results, c.checkerNodeReady(node)...)

	// 2. 检查节点调度状态
	results = append(results, c.checkNodeSchedulable(node)...)

	// 3. 检查节点内存压力
	results = append(results, c.checkNodeMemoryPressure(node)...)

	// 4. 检查节点磁盘压力
	results = append(results, c.checkNodeDiskPressure(node)...)

	// 5. 检查节点PID压力
	results = append(results, c.checkNodePIDPressure(node)...)

	// 6. 检查节点网络状态
	results = append(results, c.checkNodeNetwork(node)...)

	// 7. 检查内核版本
	results = append(results, c.checkKernelVersion(node)...)

	// 8. 检查容器运行时
	results = append(results, c.checkContainerRuntime(node)...)

	// 9. 检查操作系统
	results = append(results, c.checkOperatingSystem(node)...)

	// 10. 检查节点资源容量
	results = append(results, c.checkNodeResources(node)...)

	return results
}

func (c *NodeHealthChecker) checkerNodeReady(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	readyCondition := false
	var readyMessage string
	var lastHeartbeatTime time.Time

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			readyCondition = condition.Status == corev1.ConditionTrue
			readyMessage = condition.Message
			lastHeartbeatTime = condition.LastHeartbeatTime.Time
			break
		}
	}
	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 状态正常", nodeName)

	if !readyCondition {
		status = types.StatusFail
		severity = types.SeverityLow
		message = fmt.Sprintf("节点%s 未就绪", nodeName, readyMessage)
	}
	results = append(results, types.CheckDetail{
		Category:  c.Category(),
		CheckName: c.Name(),
		CheckID:   c.Name(),
		Status:    status,
		Message:   message,
		Severity:  severity,
		Resource:  nodeName,
		Namespace: "",
		Evidence: map[string]interface{}{
			"ready":               readyCondition,
			"mesage":              message,
			"last_heartbeat_time": lastHeartbeatTime.Format(time.RFC3339),
			"node_conditions":     node.Status.Conditions,
		},
		Suggestions: func() []string {
			if !readyCondition {
				return []string{
					"检查节点服务是否正常运行",
					"查看节点上的kubelet日志: journalctl -u kubelet",
					"检查网络连接和防火墙规则",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})
	return results
}

func (c *NodeHealthChecker) checkNodeSchedulable(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	unschedulable := node.Spec.Unschedulable
	taints := node.Spec.Taints

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 可调度", nodeName)

	var taintMessages []string
	for _, taint := range taints {
		if taint.Effect == corev1.TaintEffectNoSchedule {
			taintMessages = append(taintMessages, fmt.Sprintf("%s=%s:%s",
				taint.Key, taint.Value, taint.Effect))
		}
	}

	if unschedulable {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("节点 %s 被标记为不可调度", nodeName)
	} else if len(taintMessages) > 0 {
		status = types.StatusWarning
		severity = types.SeverityLow
		message = fmt.Sprintf("节点 %s 有调度污点: %s",
			nodeName, strings.Join(taintMessages, ", "))
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "node_schedulable",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"unschedulable": unschedulable,
			"taints":        taints,
		},
		Suggestions: func() []string {
			if unschedulable {
				return []string{"如果这是临时的维护操作，请完成后取消不可调度标记"}
			}
			if len(taintMessages) > 0 {
				return []string{"确保Pod有相应的容忍度来调度到该节点"}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkNodeMemoryPressure(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	memoryPressure := false
	var pressureMessage string

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeMemoryPressure {
			memoryPressure = condition.Status == corev1.ConditionTrue
			pressureMessage = condition.Message
			break
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 内存压力正常", nodeName)

	if memoryPressure {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("节点 %s 存在内存压力: %s", nodeName, pressureMessage)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "memory_pressure",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"memory_pressure": memoryPressure,
			"message":         pressureMessage,
		},
		Suggestions: func() []string {
			if memoryPressure {
				return []string{
					"检查是否有内存泄漏的Pod",
					"考虑增加节点内存或减少Pod内存请求",
					"检查交换空间使用情况",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkNodeDiskPressure(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	diskPressure := false
	var pressureMessage string

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeDiskPressure {
			diskPressure = condition.Status == corev1.ConditionTrue
			pressureMessage = condition.Message
			break
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 磁盘压力正常", nodeName)

	if diskPressure {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("节点 %s 存在磁盘压力: %s", nodeName, pressureMessage)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "disk_pressure",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"disk_pressure": diskPressure,
			"message":       pressureMessage,
		},
		Suggestions: func() []string {
			if diskPressure {
				return []string{
					"清理节点上的临时文件和日志",
					"检查容器日志是否过大",
					"考虑增加节点磁盘空间",
					"检查是否有大量镜像占用空间",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkNodePIDPressure(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	pidPressure := false
	var pressureMessage string

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodePIDPressure {
			pidPressure = condition.Status == corev1.ConditionTrue
			pressureMessage = condition.Message
			break
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s PID压力正常", nodeName)

	if pidPressure {
		status = types.StatusFail
		severity = types.SeverityMedium
		message = fmt.Sprintf("节点 %s 存在PID压力: %s", nodeName, pressureMessage)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "pid_pressure",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"pid_pressure": pidPressure,
			"message":      pressureMessage,
		},
		Suggestions: func() []string {
			if pidPressure {
				return []string{
					"检查是否有进程泄漏的容器",
					"调整节点PID限制",
					"重启有问题的Pod",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkNodeNetwork(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	networkUnavailable := false
	var networkMessage string

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeNetworkUnavailable {
			networkUnavailable = condition.Status == corev1.ConditionTrue
			networkMessage = condition.Message
			break
		}
	}

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 网络正常", nodeName)

	if networkUnavailable {
		status = types.StatusFail
		severity = types.SeverityHigh
		message = fmt.Sprintf("节点 %s 网络不可用: %s", nodeName, networkMessage)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "network_available",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"network_unavailable": networkUnavailable,
			"message":             networkMessage,
			"addresses":           node.Status.Addresses,
		},
		Suggestions: func() []string {
			if networkUnavailable {
				return []string{
					"检查节点网络接口配置",
					"验证CNI插件是否正常运行",
					"检查网络插件Pod状态",
					"验证节点间的网络连通性",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkKernelVersion(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name
	kernelVersion := node.Status.NodeInfo.KernelVersion

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 内核版本: %s", nodeName, kernelVersion)

	// 检查内核版本是否过旧
	if strings.Contains(kernelVersion, "3.") {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("节点 %s 使用较旧的内核版本: %s，建议升级到4.x以上",
			nodeName, kernelVersion)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "kernel_version",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"kernel_version": kernelVersion,
			"os_image":       node.Status.NodeInfo.OSImage,
			"architecture":   node.Status.NodeInfo.Architecture,
		},
		Suggestions: func() []string {
			if status == types.StatusWarning {
				return []string{
					"考虑将节点内核升级到更新的稳定版本",
					"确保新内核与当前K8s版本兼容",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkContainerRuntime(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name
	runtimeVersion := node.Status.NodeInfo.ContainerRuntimeVersion

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 容器运行时: %s", nodeName, runtimeVersion)

	// 检查是否使用Docker（Docker已被弃用）
	if strings.Contains(strings.ToLower(runtimeVersion), "docker") {
		status = types.StatusWarning
		severity = types.SeverityMedium
		message = fmt.Sprintf("节点 %s 使用Docker运行时，建议迁移到containerd", nodeName)
	}

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "container_runtime",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"container_runtime_version": runtimeVersion,
			"kubelet_version":           node.Status.NodeInfo.KubeletVersion,
		},
		Suggestions: func() []string {
			if status == types.StatusWarning {
				return []string{
					"考虑将容器运行时从Docker迁移到containerd",
					"参考K8s官方文档进行运行时迁移",
				}
			}
			return nil
		}(),
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkOperatingSystem(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name
	osImage := node.Status.NodeInfo.OSImage

	status := types.StatusPass
	severity := types.SeverityLow
	message := fmt.Sprintf("节点 %s 操作系统: %s", nodeName, osImage)

	results = append(results, types.CheckDetail{
		Category:     c.Category(),
		CheckName:    c.Name(),
		CheckID:      "operating_system",
		Status:       status,
		Message:      message,
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     severity,
		Evidence: map[string]interface{}{
			"os_image":         osImage,
			"operating_system": node.Status.NodeInfo.OperatingSystem,
		},
		Timestamp: time.Now(),
	})

	return results
}

func (c *NodeHealthChecker) checkNodeResources(node corev1.Node) []types.CheckDetail {
	var results []types.CheckDetail
	nodeName := node.Name

	// 检查资源容量
	capacity := node.Status.Capacity
	allocatable := node.Status.Allocatable

	// 检查CPU
	cpuCapacity := capacity[corev1.ResourceCPU]
	cpuAllocatable := allocatable[corev1.ResourceCPU]

	cpuCapacityQty, _ := cpuCapacity.AsInt64()
	cpuAllocatableQty, _ := cpuAllocatable.AsInt64()

	// 检查内存
	memoryCapacity := capacity[corev1.ResourceMemory]
	memoryAllocatable := allocatable[corev1.ResourceMemory]

	memoryCapacityQty := memoryCapacity.Value()
	memoryAllocatableQty := memoryAllocatable.Value()

	// 检查CPU预留是否合理
	cpuReserved := cpuCapacityQty - cpuAllocatableQty
	cpuReservedPercent := float64(cpuReserved) / float64(cpuCapacityQty) * 100

	if cpuReservedPercent > 10 {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "cpu_reservation",
			Status:       types.StatusWarning,
			Message:      fmt.Sprintf("节点 %s CPU预留较高: %.1f%%", nodeName, cpuReservedPercent),
			Resource:     nodeName,
			ResourceType: "Node",
			Namespace:    "",
			Severity:     types.SeverityLow,
			Evidence: map[string]interface{}{
				"cpu_capacity":     cpuCapacity.String(),
				"cpu_allocatable":  cpuAllocatable.String(),
				"cpu_reserved":     cpuReserved,
				"reserved_percent": cpuReservedPercent,
			},
			Suggestions: []string{
				"检查kubelet的--system-reserved和--kube-reserved配置是否合理",
				"根据节点负载调整资源预留",
			},
			Timestamp: time.Now(),
		})
	}

	// 检查内存预留是否合理
	memoryReserved := memoryCapacityQty - memoryAllocatableQty
	memoryReservedPercent := float64(memoryReserved) / float64(memoryCapacityQty) * 100

	if memoryReservedPercent > 15 {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "memory_reservation",
			Status:       types.StatusWarning,
			Message:      fmt.Sprintf("节点 %s 内存预留较高: %.1f%%", nodeName, memoryReservedPercent),
			Resource:     nodeName,
			ResourceType: "Node",
			Namespace:    "",
			Severity:     types.SeverityMedium,
			Evidence: map[string]interface{}{
				"memory_capacity":    memoryCapacity.String(),
				"memory_allocatable": memoryAllocatable.String(),
				"memory_reserved":    formatBytes(memoryReserved),
				"reserved_percent":   memoryReservedPercent,
			},
			Suggestions: []string{
				"检查kubelet的--system-reserved和--kube-reserved配置",
				"确保为系统进程留出足够内存",
				"考虑增加节点内存",
			},
			Timestamp: time.Now(),
		})
	}

	// 添加资源摘要
	results = append(results, types.CheckDetail{
		Category:  c.Category(),
		CheckName: c.Name(),
		CheckID:   "resource_summary",
		Status:    types.StatusPass,
		Message: fmt.Sprintf("节点 %s 资源容量: CPU %s, 内存 %s",
			nodeName, cpuCapacity.String(), memoryCapacity.String()),
		Resource:     nodeName,
		ResourceType: "Node",
		Namespace:    "",
		Severity:     types.SeverityLow,
		Evidence: map[string]interface{}{
			"cpu_capacity":       cpuCapacity.String(),
			"cpu_allocatable":    cpuAllocatable.String(),
			"memory_capacity":    memoryCapacity.String(),
			"memory_allocatable": memoryAllocatable.String(),
			"pods_capacity": func() string {
				if p := capacity[corev1.ResourcePods]; p.IsZero() {
					return "0"
				} else {
					return p.String()
				}
			}(),
		},
		Timestamp: time.Now(),
	})

	return results
}

// 辅助函数：格式化字节大小
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(bytes)/float64(div), "KMGTPE"[exp])
}

// 注册检查器
func init() {
	Register(&NodeHealthChecker{})
}
