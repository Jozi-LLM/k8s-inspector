package main

import (
	"fmt"

	"github.com/ym/k8s-inspector/internal/checker"
)

func main() {
	// 初始化检查器
	checker.InitCheckers()

	// 调试：打印registry的内容
	checker.DebugPrintRegistry()

	// 打印注册的检查器列表
	registeredCheckers := checker.List()
	fmt.Printf("已注册的检查器数量: %d\n", len(registeredCheckers))
	for _, c := range registeredCheckers {
		fmt.Printf("已注册的检查器: %s\n", c.Name())
	}
}
