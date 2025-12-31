// internal/alerting/slack_sender.go
package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ym/k8s-inspector/internal/pkg/types"
)

// Slack发送器
type SlackSender struct {
	webhookURL string
	enabled    bool
	channel    string
	username   string
	iconEmoji  string
}

// 创建Slack发送器
func NewSlackSender(webhookURL, channel, username string) *SlackSender {
	return &SlackSender{
		webhookURL: webhookURL,
		enabled:    webhookURL != "",
		channel:    channel,
		username:   username,
		iconEmoji:  ":warning:",
	}
}

func (s *SlackSender) Name() string {
	return "slack"
}

func (s *SlackSender) SupportsLevel(level AlertLevel) bool {
	// Slack支持所有级别
	return s.enabled
}

func (s *SlackSender) Send(ctx context.Context, alert *AlertMessage) error {
	if !s.enabled {
		return fmt.Errorf("Slack发送器未启用")
	}

	// 构建Slack消息
	slackMessage := s.buildSlackMessage(alert)

	// 转换为JSON
	payload, err := json.Marshal(slackMessage)
	if err != nil {
		return fmt.Errorf("序列化Slack消息失败: %v", err)
	}

	// 发送HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送Slack消息失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Slack返回错误状态码: %d", resp.StatusCode)
	}

	return nil
}

// 构建Slack消息
func (s *SlackSender) buildSlackMessage(alert *AlertMessage) map[string]interface{} {
	// 颜色根据告警级别
	color := "#36a64f" // 绿色 - INFO
	switch alert.Level {
	case AlertLevelWarning:
		color = "#ffcc00" // 黄色
	case AlertLevelError:
		color = "#ff9900" // 橙色
	case AlertLevelCritical:
		color = "#ff0000" // 红色
	}

	// 表情根据严重程度
	emoji := ":information_source:"
	switch alert.Severity {
	case types.SeverityMedium:
		emoji = ":warning:"
	case types.SeverityHigh:
		emoji = ":exclamation:"
	case types.SeverityCritical:
		emoji = ":rotating_light:"
	}

	// 构建消息字段
	fields := []map[string]interface{}{
		{
			"title": "集群",
			"value": alert.Cluster,
			"short": true,
		},
		{
			"title": "级别",
			"value": string(alert.Level),
			"short": true,
		},
	}

	if alert.Resource != "" {
		fields = append(fields, map[string]interface{}{
			"title": "资源",
			"value": alert.Resource,
			"short": true,
		})
	}

	if alert.Namespace != "" {
		fields = append(fields, map[string]interface{}{
			"title": "命名空间",
			"value": alert.Namespace,
			"short": true,
		})
	}

	// 添加建议
	if suggestions, ok := alert.Details["suggestions"].([]string); ok && len(suggestions) > 0 {
		fields = append(fields, map[string]interface{}{
			"title": "建议",
			"value": strings.Join(suggestions, "\n"),
			"short": false,
		})
	}

	message := map[string]interface{}{
		"channel":    s.channel,
		"username":   s.username,
		"icon_emoji": s.iconEmoji,
		"attachments": []map[string]interface{}{
			{
				"color":       color,
				"title":       fmt.Sprintf("%s %s", emoji, alert.Title),
				"text":        alert.Message,
				"fields":      fields,
				"ts":          alert.Timestamp.Unix(),
				"footer":      "K8s巡检工具",
				"footer_icon": "https://kubernetes.io/images/favicon.png",
			},
		},
	}

	return message
}
