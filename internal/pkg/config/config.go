package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/spf13/viper"
)

type Config struct {
	Inspector InspectorConfig `yaml:"inspector"`
	Logging   LoggingConfig   `yaml:"logging"`
	Alerting  AlertingConfig  `yaml:"alerting"`
	Storage   StorageConfig   `yaml:"storage"`
	Server    ServerConfig    `yaml:"server"`
}

type InspectorConfig struct {
	Schedule  string       `yaml:"schedule"`
	Clusters  []Cluster    `yaml:"clusters"`
	Checks    CheckConfig  `yaml:"checks"`
	Reporting ReportConfig `yaml:"reporting"`
}

type Cluster struct {
	Name       string `yaml:"name"`
	Kubeconfig string `yaml:"kubeconfig"`
	Context    string `yaml:"context"`
}

type CheckConfig struct {
	Enabled    []string          `yaml:"enabled"`
	Thresholds map[string]int    `yaml:"thresholds"`
	Settings   map[string]string `yaml:"settings"`
}

type ReportConfig struct {
	Formats   []string `yaml:"formats"`
	OutputDir string   `yaml:"output_dir"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
	File   string `yaml:"file"`
}

type AlertingConfig struct {
	Slack   SlackConfig   `yaml:"slack"`
	Webhook WebhookConfig `yaml:"webhook"`
	Email   EmailConfig   `yaml:"email"`
}

type SlackConfig struct {
	Enabled bool   `yaml:"enabled"`
	Webhook string `yaml:"webhook"`
	Channel string `yaml:"channel"`
}

type WebhookConfig struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
}

type EmailConfig struct {
	Enabled    bool     `yaml:"enabled"`
	SMTPServer string   `yaml:"smtp_server"`
	Recipients []string `yaml:"recipients"`
}

type StorageConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Driver   string `yaml:"driver"`
	DSN      string `yaml:"dsn"`
	MaxConns int    `yaml:"max_conns"`
}

type ServerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
}

// 添加默认配置
func DefaultConfig() *Config {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	kubeconfig := ""
	if home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	return &Config{
		Inspector: InspectorConfig{
			Schedule: "0 */6 * * *",
			Clusters: []Cluster{
				{
					Name:       "default",
					Kubeconfig: kubeconfig,
					Context:    "",
				},
			},
			Reporting: ReportConfig{
				Formats:   []string{"console", "html"},
				OutputDir: "./reports",
			},
			Checks: CheckConfig{
				Enabled: []string{
					"node_health",
					"resource_usage",
					"pod_health",
				},
				Thresholds: map[string]int{
					"cpu_usage_warning":    60,
					"memory_usage_warning": 60,
					"pod_restart_warning":  3,
				},
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
			File:   "./logs/k8s-inspector.log",
		},
	}
}

// 加载配置
func LoadConfig(configPath string) (*Config, error) {
	// 创建默认配置
	cfg := DefaultConfig()

	// 如果默认配置文件不存在，创建默认配置文件
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := SaveDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("创建默认配置文件失败: %w", err)
		}
	}

	// 读取配置文件
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 绑定配置到结构体
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("绑定配置到结构体失败: %w", err)
	}
	return cfg, nil
}

// 保存默认配置
func SaveDefaultConfig(configPath string) error {
	// 确保目录存在
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 创建配置文件
	cfg := DefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}
