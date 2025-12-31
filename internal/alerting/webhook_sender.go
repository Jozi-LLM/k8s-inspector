// internal/alerting/webhook_sender.go
package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Webhook发送器
type WebhookSender struct {
	url     string
	enabled bool
	headers map[string]string
}

// 创建Webhook发送器
func NewWebhookSender(url string, headers map[string]string) *WebhookSender {
	return &WebhookSender{
		url:     url,
		enabled: url != "",
		headers: headers,
	}
}

func (s *WebhookSender) Name() string {
	return "webhook"
}

func (s *WebhookSender) SupportsLevel(level AlertLevel) bool {
	return s.enabled
}

func (s *WebhookSender) Send(ctx context.Context, alert *AlertMessage) error {
	if !s.enabled {
		return fmt.Errorf("Webhook发送器未启用")
	}

	// 构建请求体
	payload := map[string]interface{}{
		"id":        alert.ID,
		"level":     alert.Level,
		"title":     alert.Title,
		"message":   alert.Message,
		"cluster":   alert.Cluster,
		"check":     alert.CheckName,
		"resource":  alert.Resource,
		"namespace": alert.Namespace,
		"severity":  alert.Severity,
		"timestamp": alert.Timestamp.Format(time.RFC3339),
		"details":   alert.Details,
	}

	// 转换为JSON
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化Webhook消息失败: %v", err)
	}

	// 发送HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", s.url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// 添加自定义头部
	for key, value := range s.headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送Webhook请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook返回错误状态码: %d", resp.StatusCode)
	}

	return nil
}
