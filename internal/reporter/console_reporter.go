package reporter

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/ym/k8s-inspector/internal/pkg/types"
)

// Reporter 定义报告生成器接口
type Reporter interface {
	Generate(result *types.InspectionResult) error
}

type ConsoleReporter struct{}

func NewConsoleReporter() *ConsoleReporter {
	return &ConsoleReporter{}
}

func (r *ConsoleReporter) Generate(result *types.InspectionResult) error {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("Kubernetes集群巡检报告")
	fmt.Println(strings.Repeat("=", 80))

	// 集群信息
	fmt.Printf("\n集群: %s\n", result.ClusterName)
	fmt.Printf("时间: %s\n", result.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("耗时: %.2f秒\n", result.Duration.Seconds())

	//摘要信息
	summary := result.Summary
	fmt.Printf("\n%s\n", strings.Repeat("-", 80))
	fmt.Println("检查摘要:")
	fmt.Printf("总分: %d/100\n", summary.Score)
	fmt.Printf("检查项: 总计%d 通过%d 警告%d 失败%d 错误%d 跳过%d\n",
		summary.TotalChecks, summary.Passed, summary.Warnings,
		summary.Failed, summary.Errors, summary.Skipped)
	fmt.Println(strings.Repeat("-", 80))

	// 按分组类别显示
	resultsByCategory := make(map[string][]types.CheckDetail)
	for _, detail := range result.Details {
		resultsByCategory[detail.Category] = append(resultsByCategory[detail.Category], detail)
	}

	for category, details := range resultsByCategory {
		fmt.Printf("\n%s:\n", strings.ToUpper(category))
		fmt.Println(strings.Repeat("-", 40))

		// 处理node_health检查结果的合并显示
		if category == "node" {
			r.printMergedNodeHealthResults(details)
		} else if category == "workload" {
			// 处理工作负载健康检查结果的合并显示
			r.printMergedWorkloadResults(details)
		} else {
			// 其他类别的检查结果保持原有显示方式
			for _, detail := range details {
				statusColor := r.getStatusColor(detail.Status)
				severityIcon := r.getSeverityIcon(detail.Severity)

				fmt.Printf("%s %s %s %s\n",
					severityIcon,
					statusColor.Sprintf("%-10s", detail.Status),
					detail.CheckName,
					detail.Message)
				if len(detail.Suggestions) > 0 {
					for _, suggestion := range detail.Suggestions {
						fmt.Printf("  建议: %s\n", suggestion)
					}
				}
			}
		}
	}

	// 推荐建议 (暂时隐藏)
	// if len(result.Recommendations) > 0 {
	// 	fmt.Println("\n%s \n", strings.Repeat("-", 80))
	// 	fmt.Println("推荐建议:")
	// 	for i, rec := range result.Recommendations {
	// 		fmt.Printf("%d. %s\n", i+1, rec)
	// 	}
	// }
	fmt.Println(strings.Repeat("=", 80))
	return nil
}

func (r *ConsoleReporter) getStatusColor(status types.CheckStatus) *color.Color {
	switch status {
	case types.StatusPass:
		return color.New(color.FgGreen)
	case types.StatusWarning:
		return color.New(color.FgYellow)
	case types.StatusFail:
		return color.New(color.FgRed)
	case types.StatusError:
		return color.New(color.FgMagenta)
	default:
		return color.New(color.FgWhite)
	}
}

func (r *ConsoleReporter) getSeverityIcon(severity types.SeverityLevel) string {
	switch severity {
	case types.SeverityLow:
		return "📊"
	case types.SeverityMedium:
		return "⚠️"
	case types.SeverityHigh:
		return "🚨"
	case types.SeverityCritical:
		return "🔥"
	default:
		return "📝"
	}
}

// printMergedNodeHealthResults 合并显示节点健康检查结果
func (r *ConsoleReporter) printMergedNodeHealthResults(details []types.CheckDetail) {
	// 按节点名称分组
	nodeResults := make(map[string][]types.CheckDetail)
	for _, detail := range details {
		if detail.CheckName == "node_health" {
			nodeResults[detail.Resource] = append(nodeResults[detail.Resource], detail)
		} else {
			// 非node_health的检查结果保持原有显示方式
			statusColor := r.getStatusColor(detail.Status)
			severityIcon := r.getSeverityIcon(detail.Severity)
			fmt.Printf("%s %s %s %s\n",
				severityIcon,
				statusColor.Sprintf("%-10s", detail.Status),
				detail.CheckName,
				detail.Message)
			if len(detail.Suggestions) > 0 {
				for _, suggestion := range detail.Suggestions {
					fmt.Printf("  建议: %s\n", suggestion)
				}
			}
		}
	}

	// 合并显示每个节点的健康检查结果
	for nodeName, nodeDetails := range nodeResults {
		if len(nodeDetails) == 0 {
			continue
		}

		// 确定节点的整体状态
		overallStatus := types.StatusPass
		overallSeverity := types.SeverityLow
		for _, detail := range nodeDetails {
			if detail.Status > overallStatus {
				overallStatus = detail.Status
			}
			if detail.Severity > overallSeverity {
				overallSeverity = detail.Severity
			}
		}

		// 显示节点整体状态
		statusColor := r.getStatusColor(overallStatus)
		severityIcon := r.getSeverityIcon(overallSeverity)
		fmt.Printf("%s %s %s %s\n",
			severityIcon,
			statusColor.Sprintf("%-10s", overallStatus),
			"node_health",
			fmt.Sprintf("节点 %s 健康状态", nodeName))

		// 显示详细的检查结果
		for _, detail := range nodeDetails {
			detailStatusColor := r.getStatusColor(detail.Status)
			// 只显示消息的关键部分，去除节点名称前缀
			message := detail.Message
			if strings.HasPrefix(message, "节点 "+nodeName+" ") {
				message = strings.TrimPrefix(message, "节点 "+nodeName+" ")
			}
			fmt.Printf("  %s %s\n",
				detailStatusColor.Sprintf("%-10s", detail.Status),
				message)

			if len(detail.Suggestions) > 0 {
				for _, suggestion := range detail.Suggestions {
					fmt.Printf("    建议: %s\n", suggestion)
				}
			}
		}
	}
}

// printMergedWorkloadResults 合并显示工作负载健康检查结果
func (r *ConsoleReporter) printMergedWorkloadResults(details []types.CheckDetail) {
	// 按检查器名称分组
	checkerResults := make(map[string][]types.CheckDetail)
	for _, detail := range details {
		checkerResults[detail.CheckName] = append(checkerResults[detail.CheckName], detail)
	}

	// 处理pod_health检查结果
	if podDetails, ok := checkerResults["pod_health"]; ok {
		// 按Pod名称分组
		podResults := make(map[string][]types.CheckDetail)
		for _, detail := range podDetails {
			podKey := detail.Resource
			if detail.Namespace != "" {
				podKey = detail.Namespace + "/" + detail.Resource
			}
			podResults[podKey] = append(podResults[podKey], detail)
		}

		// 合并显示每个Pod的健康检查结果
		for podKey, podDetails := range podResults {
			if len(podDetails) == 0 {
				continue
			}

			// 确定Pod的整体状态
			overallStatus := types.StatusPass
			overallSeverity := types.SeverityLow
			for _, detail := range podDetails {
				if detail.Status > overallStatus {
					overallStatus = detail.Status
				}
				if detail.Severity > overallSeverity {
					overallSeverity = detail.Severity
				}
			}

			// 显示Pod整体状态
			statusColor := r.getStatusColor(overallStatus)
			severityIcon := r.getSeverityIcon(overallSeverity)
			fmt.Printf("%s %s %s Pod %s 状态\n",
				severityIcon,
				statusColor.Sprintf("%-10s", overallStatus),
				"pod_health",
				podKey)

			// 显示详细的检查结果
			for _, detail := range podDetails {
				detailStatusColor := r.getStatusColor(detail.Status)
				// 只显示消息的关键部分，去除Pod名称前缀
				message := detail.Message
				if strings.HasPrefix(message, "Pod "+podKey+" ") {
					message = strings.TrimPrefix(message, "Pod "+podKey+" ")
				}
				fmt.Printf("  %s %s\n",
					detailStatusColor.Sprintf("%-10s", detail.Status),
					message)

				if len(detail.Suggestions) > 0 {
					for _, suggestion := range detail.Suggestions {
						fmt.Printf("    建议: %s\n", suggestion)
					}
				}
			}
		}
	}

	// 处理其他检查器的结果（如deployment_health）
	for checkerName, checkerDetails := range checkerResults {
		if checkerName != "pod_health" {
			// 保持原有显示方式
			for _, detail := range checkerDetails {
				statusColor := r.getStatusColor(detail.Status)
				severityIcon := r.getSeverityIcon(detail.Severity)
				fmt.Printf("%s %s %s %s\n",
					severityIcon,
					statusColor.Sprintf("%-10s", detail.Status),
					detail.CheckName,
					detail.Message)
				if len(detail.Suggestions) > 0 {
					for _, suggestion := range detail.Suggestions {
						fmt.Printf("  建议: %s\n", suggestion)
					}
				}
			}
		}
	}
}
