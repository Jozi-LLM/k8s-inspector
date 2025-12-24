package base

import (
	"context"
	"go/types"

	"github.com/ym/k8s-inspector/internal/pkg/k8s"
)

type CluserInfoChecker struct{}

func (c *CluserInfoChecker) Name() string {
	return "cluser_info"
}

func (c *CluserInfoChecker) Description() string {
	return "检查集群基本信息和版本"
}

func (c *CluserInfoChecker) Category() string {
	return "base"
}

func (c *CluserInfoChecker) RequiredPermissions() []string {
	return []string{"get", "list"}
}

func (c *CluserInfoChecker) Execute(ctx context.Context, clent *k8s.Client) ([]types.CheckerDetail, error)
	logger := utils.GetLogger()
	logger.Infow("开始进行集群基本信息检查", "checker", c.Name())

	startTime := time.Now()
	var result []types.CheckerDetail

	//检查集群链接
	info, err := client.GetClusterInfo(ctx)
	if err != nil {
		results = append(results, types.CheckerDetail{
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
	if ok{
		results = append(results, types.CheckerDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "cluster_version",
			Status:    types.StatusPass,
			Message:   fmt.Sprintf("集群版本: %v", version),
			Resource:  "cluster",
			ResourceType:"cluster",
			Severity:    types.SeverityLow,
			Evidence:    info,
			Timestamp:   time.Now(),
		})
	}

	// 获取节点数量
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, types.CheckerDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "cluster_node_count",
			Status:    types.StatusFail,
			Message:   fmt.Sprintf("获取集群节点数量失败: %v", err),
			Severity:  types.SeverityCritical,
			Timestamp: time.Now(),
		})
	}else{
		readyNotes := 0
		for _, node := range nodes.Items{
			for _, condition := range node.Status.Conditions{
				if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue{
					readyNotes++
					break
				}
			}
		}

		results = append(results, types.CheckerDetail{
			Category:  c.Category(),
			CheckName: c.Name(),
			CheckID:   "cluster_node_count",
			Status:    func() types.Status {
				if readyNotes == len(nodes.Items){
					return types.StatusPass
				} else if readyNotes == 0{
					return types.StatusWarning
				} else {
					return types.StatusFail
				}(),
				Message:      fmt.Sprintf("节点状态: %d/%d Ready", readyNodes, len(nodes.Items)),
				Resource: "cluster",
				ResourceType:"cluster",
				Severity:   func() types.Severity {
					if readyNodes == 0 {
						return types.SeverityCritical
					} else if readyNodes < len(nodes.Items) {
						return types.SeverityMedium
					}
					return types.SeverityLow
					}(),
					Evidence: map[string]interface{}{
						"readyNodes": readyNodes,
						"totalNodes": len(nodes.Items),
					},
					Suggestions: []string{
						if readyNodes < len(nodes.Items) {
							return fmt.Sprintf("建议检查节点状态，当前有 %d 个节点未就绪", len(nodes.Items)-readyNodes)
						}
						return nil
					}(),
					Timestamp: time.Now(),
				})
			}

			duration := time.Since(startTime)
			logger.Infow("集群基本信息检查完成", 
				"checker", c.Name(), 
				"duration", duration,
				"results",len(results))
			return results, nil
	}

// 注册检查器
func init() {
	checker.Register(&ClusterInfoChecker{})
}