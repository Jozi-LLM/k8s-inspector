// internal/analyzer/engine.go
package analyzer

import (
	"context"
	"fmt"
	"time"

	"github.com/ym/k8s-inspector/internal/alerting"
	"github.com/ym/k8s-inspector/internal/checker"
	"github.com/ym/k8s-inspector/internal/pkg/config"
	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

type InspectionEngine struct {
	config       *config.InspectorConfig
	checkers     []string
	alertManager *alerting.AlertManager
}

func NewInspectionEngine(config *config.InspectorConfig, alertManager *alerting.AlertManager) *InspectionEngine {
	return &InspectionEngine{
		config:       config,
		checkers:     config.Checks.Enabled,
		alertManager: alertManager,
	}
}

func (e *InspectionEngine) Run(ctx context.Context, client *k8s.Client, clusterName string) (*types.InspectionResult, error) {
	logger := utils.GetGlobalLogger()
	startTime := time.Now()

	logger.Infow("开始集群巡检",
		"cluster", clusterName,
		"checkers", len(e.checkers))

	// 执行所有检查器
	details, err := checker.ExecuteAll(ctx, client, e.checkers)
	if err != nil {
		return nil, fmt.Errorf("执行检查器失败: %v", err)
	}

	// 计算摘要
	summary := calculateSummary(details)

	// 计算健康分
	summary.Score = calculateHealthScore(details)

	// 生成建议
	recommendations := generateRecommendations(details)

	// 获取集群信息
	clusterInfo, _ := client.GetClusterInfo(ctx)

	result := &types.InspectionResult{
		ClusterName:     clusterName,
		ClusterInfo:     clusterInfo,
		Timestamp:       time.Now(),
		Duration:        time.Since(startTime),
		Summary:         summary,
		Details:         details,
		Recommendations: recommendations,
		Metadata: types.Metadata{
			ToolVersion: "1.0.0",
			GeneratedBy: "k8s-inspector",
		},
	}

	logger.Infow("集群巡检完成",
		"cluster", clusterName,
		"duration", result.Duration,
		"score", summary.Score,
		"passed", summary.Passed,
		"failed", summary.Failed)

	return result, nil
}

func calculateSummary(details []types.CheckDetail) types.Summary {
	var summary types.Summary
	summary.TotalChecks = len(details)

	for _, detail := range details {
		switch detail.Status {
		case types.StatusPass:
			summary.Passed++
		case types.StatusWarning:
			summary.Warnings++
		case types.StatusFail:
			summary.Failed++
		case types.StatusError:
			summary.Errors++
		case types.StatusSkipped:
			summary.Skipped++
		}
	}

	return summary
}

func calculateHealthScore(details []types.CheckDetail) int {
	if len(details) == 0 {
		return 100
	}

	totalWeight := 0
	weightedScore := 0

	for _, detail := range details {
		weight := getCheckWeight(detail.Severity, detail.Status)
		score := getStatusScore(detail.Status)

		totalWeight += weight
		weightedScore += score * weight
	}

	if totalWeight == 0 {
		return 100
	}

	return weightedScore / totalWeight
}

func getCheckWeight(severity types.SeverityLevel, status types.CheckStatus) int {
	baseWeight := 1

	switch severity {
	case types.SeverityCritical:
		baseWeight = 5
	case types.SeverityHigh:
		baseWeight = 3
	case types.SeverityMedium:
		baseWeight = 2
	case types.SeverityLow:
		baseWeight = 1
	}

	// 失败项的权重加倍
	if status == types.StatusFail || status == types.StatusError {
		return baseWeight * 2
	}

	return baseWeight
}

func getStatusScore(status types.CheckStatus) int {
	switch status {
	case types.StatusPass:
		return 100
	case types.StatusWarning:
		return 70
	case types.StatusSkipped:
		return 50
	case types.StatusFail:
		return 30
	case types.StatusError:
		return 0
	default:
		return 50
	}
}

func generateRecommendations(details []types.CheckDetail) []string {
	var recommendations []string

	// 按严重程度排序
	criticalFails := 0
	highFails := 0

	for _, detail := range details {
		if detail.Status == types.StatusFail {
			switch detail.Severity {
			case types.SeverityCritical:
				criticalFails++
			case types.SeverityHigh:
				highFails++
			}
		}
	}

	if criticalFails > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("发现 %d 个严重问题，请立即处理", criticalFails))
	}

	if highFails > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("发现 %d 个高危问题，建议尽快处理", highFails))
	}

	// 从检查结果中提取建议
	for _, detail := range details {
		if len(detail.Suggestions) > 0 &&
			(detail.Status == types.StatusFail || detail.Status == types.StatusWarning) {
			recommendations = append(recommendations, detail.Suggestions...)
		}
	}

	// 去重
	uniqueRecs := make(map[string]bool)
	var finalRecs []string
	for _, rec := range recommendations {
		if !uniqueRecs[rec] {
			uniqueRecs[rec] = true
			finalRecs = append(finalRecs, rec)
		}
	}

	return finalRecs
}
