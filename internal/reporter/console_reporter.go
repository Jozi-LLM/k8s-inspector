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

	// 推荐建议
	if len(result.Recommendations) > 0 {
		fmt.Println("\n%s \n", strings.Repeat("-", 80))
		fmt.Println("推荐建议:")
		for i, rec := range result.Recommendations {
			fmt.Printf("%d. %s\n", i+1, rec)
		}
	}
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
