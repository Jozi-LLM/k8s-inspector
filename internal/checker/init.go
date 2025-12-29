package checker

import (
	"github.com/ym/k8s-inspector/internal/checker/workload"
)

// InitCheckers 初始化并注册所有检查器
func InitCheckers() {
	// 注册节点健康检查器
	Register(&NodeHealthChecker{})
	
	// 注册Pod健康检查器
	Register(&workload.PodHealthChecker{})
	
	// TODO: 添加其他检查器
	// Register(&ResourceUsageChecker{})
	// Register(&DeploymentHealthChecker{})
	// Register(&SecurityBasicChecker{})
}