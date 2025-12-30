package checker

import (
	"github.com/ym/k8s-inspector/internal/checker/resource"
	"github.com/ym/k8s-inspector/internal/checker/workload"
)

// InitCheckers 初始化并注册所有检查器
func InitCheckers() {
	// 注册节点健康检查器
	Register(&NodeHealthChecker{})

	// 注册Pod健康检查器
	Register(&workload.PodHealthChecker{})

	// 注册Deployment健康检查器
	Register(&workload.DeploymentChecker{})

	// 注册资源使用检查器
	Register(&resource.ResourceUsageChecker{})
}
