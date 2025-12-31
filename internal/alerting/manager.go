// internal/alerting/manager.go
package alerting

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ym/k8s-inspector/internal/pkg/config"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

// 告警级别
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "INFO"
	AlertLevelWarning  AlertLevel = "WARNING"
	AlertLevelError    AlertLevel = "ERROR"
	AlertLevelCritical AlertLevel = "CRITICAL"
)

// 告警消息
type AlertMessage struct {
	ID        string
	Level     AlertLevel
	Title     string
	Message   string
	Cluster   string
	CheckName string
	Resource  string
	Namespace string
	Severity  types.SeverityLevel
	Timestamp time.Time
	Details   map[string]interface{}
}

// 告警发送器接口
type AlertSender interface {
	Name() string
	Send(ctx context.Context, alert *AlertMessage) error
	SupportsLevel(level AlertLevel) bool
}

// 告警管理器
type AlertManager struct {
	senders   map[string]AlertSender
	config    *config.AlertingConfig
	logger    *utils.Logger
	mutex     sync.RWMutex
	alerts    []*AlertMessage
	maxAlerts int
}

// 创建告警管理器
func NewAlertManager(cfg *config.AlertingConfig) *AlertManager {
	return &AlertManager{
		senders:   make(map[string]AlertSender),
		config:    cfg,
		logger:    utils.GetGlobalLogger(),
		alerts:    make([]*AlertMessage, 0),
		maxAlerts: 1000, // 最多保存1000条告警
	}
}

// 注册告警发送器
func (m *AlertManager) RegisterSender(sender AlertSender) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.senders[sender.Name()] = sender
	m.logger.Infow("注册告警发送器", "sender", sender.Name())
}

// 发送告警
func (m *AlertManager) SendAlert(ctx context.Context, alert *AlertMessage) error {
	m.mutex.Lock()
	// 保存告警记录
	m.alerts = append(m.alerts, alert)
	// 限制告警数量
	if len(m.alerts) > m.maxAlerts {
		m.alerts = m.alerts[len(m.alerts)-m.maxAlerts:]
	}
	m.mutex.Unlock()

	m.logger.Infow("发送告警",
		"level", alert.Level,
		"title", alert.Title,
		"cluster", alert.Cluster)

	// 并行发送到所有发送器
	var wg sync.WaitGroup
	var errors []string

	m.mutex.RLock()
	senders := make([]AlertSender, 0, len(m.senders))
	for _, sender := range m.senders {
		if sender.SupportsLevel(alert.Level) {
			senders = append(senders, sender)
		}
	}
	m.mutex.RUnlock()

	for _, sender := range senders {
		wg.Add(1)
		go func(s AlertSender) {
			defer wg.Done()

			if err := s.Send(ctx, alert); err != nil {
				m.logger.Errorw("发送告警失败",
					"sender", s.Name(),
					"error", err)
				errors = append(errors, fmt.Sprintf("%s: %v", s.Name(), err))
			} else {
				m.logger.Debugw("告警发送成功", "sender", s.Name())
			}
		}(sender)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("告警发送失败: %s", strings.Join(errors, "; "))
	}

	return nil
}

// 从巡检结果生成告警
func (m *AlertManager) SendAlertsFromInspection(ctx context.Context, result *types.InspectionResult) error {
	// 根据严重程度生成告警
	var alerts []*AlertMessage

	for _, detail := range result.Details {
		if detail.Status == types.StatusFail || detail.Status == types.StatusError {
			alertLevel := m.severityToAlertLevel(detail.Severity)

			alert := &AlertMessage{
				ID:        fmt.Sprintf("%s-%d", result.ClusterName, time.Now().UnixNano()),
				Level:     alertLevel,
				Title:     fmt.Sprintf("[%s] %s 检查失败", result.ClusterName, detail.CheckName),
				Message:   detail.Message,
				Cluster:   result.ClusterName,
				CheckName: detail.CheckName,
				Resource:  detail.Resource,
				Namespace: detail.Namespace,
				Severity:  detail.Severity,
				Timestamp: time.Now(),
				Details: map[string]interface{}{
					"category":    detail.Category,
					"evidence":    detail.Evidence,
					"suggestions": detail.Suggestions,
					"score":       result.Summary.Score,
				},
			}

			alerts = append(alerts, alert)
		}
	}

	// 如果健康分过低，发送总体告警
	if result.Summary.Score < 60 {
		alert := &AlertMessage{
			ID:        fmt.Sprintf("%s-score-%d", result.ClusterName, time.Now().UnixNano()),
			Level:     AlertLevelCritical,
			Title:     fmt.Sprintf("[%s] 集群健康分过低: %d", result.ClusterName, result.Summary.Score),
			Message:   fmt.Sprintf("集群 %s 健康分仅为 %d，存在严重问题", result.ClusterName, result.Summary.Score),
			Cluster:   result.ClusterName,
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"score":           result.Summary.Score,
				"total_checks":    result.Summary.TotalChecks,
				"passed":          result.Summary.Passed,
				"failed":          result.Summary.Failed,
				"warnings":        result.Summary.Warnings,
				"recommendations": result.Recommendations,
			},
		}
		alerts = append(alerts, alert)
	}

	// 发送所有告警
	for _, alert := range alerts {
		if err := m.SendAlert(ctx, alert); err != nil {
			m.logger.Errorw("发送巡检告警失败", "error", err)
		}
	}

	return nil
}

// 严重程度转告警级别
func (m *AlertManager) severityToAlertLevel(severity types.SeverityLevel) AlertLevel {
	switch severity {
	case types.SeverityCritical:
		return AlertLevelCritical
	case types.SeverityHigh:
		return AlertLevelError
	case types.SeverityMedium:
		return AlertLevelWarning
	default:
		return AlertLevelInfo
	}
}

// 获取历史告警
func (m *AlertManager) GetAlerts(limit int) []*AlertMessage {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if limit <= 0 || limit > len(m.alerts) {
		limit = len(m.alerts)
	}

	// 返回最新的告警
	start := len(m.alerts) - limit
	if start < 0 {
		start = 0
	}

	alerts := make([]*AlertMessage, limit)
	copy(alerts, m.alerts[start:])

	return alerts
}
