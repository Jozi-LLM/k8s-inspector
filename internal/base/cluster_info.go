package base

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

type ClusterInfoChecker struct{}

func (c *ClusterInfoChecker) Name() string {
	return "cluster_info"
}

func (c *ClusterInfoChecker) Description() string {
	return "检查集群基本信息和版本"
}

func (c *ClusterInfoChecker) Category() string {
	return "base"
}

func (c *ClusterInfoChecker) RequiredPermissions() []string {
	return []string{"get", "list"}
}

func (c *ClusterInfoChecker) Execute(ctx context.Context, client *k8s.Client) ([]types.CheckDetail, error) {
	logger := utils.GetGlobalLogger()
	logger.Infow("开始进行集群基本信息检查", "checker", c.Name())

	startTime := time.Now()
	var results []types.CheckDetail

	// 检查集群连接
	info, err := client.GetClusterInfo(ctx)
	if err != nil {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "cluster_connectivity",
			Status:    types.StatusFail,
			Message:   fmt.Sprintf("集群连接失败: %v", err),
			Severity:  types.SeverityCritical,
			Timestamp: time.Now(),
		})
		return results, nil
	}

	// 检查集群版本
	version, ok := info["version"]
	if ok {
		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "cluster_version",
			Status:       types.StatusPass,
			Message:      fmt.Sprintf("集群版本: %v", version),
			Resource:     "cluster",
			ResourceType: "cluster",
			Severity:     types.SeverityLow,
			Evidence:     info,
			Timestamp:    time.Now(),
		})
	}

	// 获取节点数量
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, types.CheckDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "cluster_node_count",
			Status:    types.StatusFail,
			Message:   fmt.Sprintf("获取集群节点数量失败: %v", err),
			Severity:  types.SeverityCritical,
			Timestamp: time.Now(),
		})
	} else {
		readyNodes := 0
		for _, node := range nodes.Items {
			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					readyNodes++
					break
				}
			}
		}

		status := types.StatusPass
		if readyNodes == 0 {
			status = types.StatusWarning
		} else if readyNodes < len(nodes.Items) {
			status = types.StatusFail
		}

		severity := types.SeverityLow
		if readyNodes == 0 {
			severity = types.SeverityCritical
		} else if readyNodes < len(nodes.Items) {
			severity = types.SeverityMedium
		}

		message := fmt.Sprintf("节点状态: %d/%d Ready", readyNodes, len(nodes.Items))
		suggestions := []string{}
		if readyNodes < len(nodes.Items) {
			suggestions = append(suggestions, fmt.Sprintf("建议检查节点状态，当前有 %d 个节点未就绪", len(nodes.Items)-readyNodes))
		}

		results = append(results, types.CheckDetail{
			Category:     c.Category(),
			CheckName:    c.Name(),
			CheckID:      "cluster_node_count",
			Status:       status,
			Message:      message,
			Resource:     "cluster",
			ResourceType: "cluster",
			Severity:     severity,
			Evidence: map[string]interface{}{
				"readyNodes": readyNodes,
				"totalNodes": len(nodes.Items),
			},
			Suggestions: suggestions,
			Timestamp:   time.Now(),
		})
	}

	duration := time.Since(startTime)
	logger.Infow("集群基本信息检查完成",
		"checker", c.Name(),
		"duration", duration,
		"results", len(results))

	return results, nil
}