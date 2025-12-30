package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/k8s-inspector/internal/analyzer"
	"github.com/ym/k8s-inspector/internal/pkg/config"
	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
	"github.com/ym/k8s-inspector/internal/scheduler"
	"github.com/ym/k8s-inspector/internal/storage"
)

var (
	configPath = flag.String("config", "./configs/config.yaml", "配置文件路径")
	mode       = flag.String("mode", "inspect", "运行模式: inspect(单次巡检)/serve(服务模式)")
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	if err := utils.InitGlobalLogger(&utils.LogConfig{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: cfg.Logging.Output,
		File:   "./logs/k8s-inspector.log",
	}); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}

	logger := utils.GetLogger()
	defer logger.Sync()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		logger.Info("收到终止信号，正在关闭...")
		cancel()
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()

	// 根据模式运行
	switch *mode {
	case "inspect":
		if err := runInspection(ctx, cfg); err != nil {
			logger.Fatalw("巡检执行失败", "error", err)
		}
	case "serve":
		if err := runServer(ctx, cfg); err != nil {
			logger.Fatalw("服务运行失败", "error", err)
		}
	default:
		logger.Fatalf("未知模式: %s", *mode)
	}
}

func runInspection(ctx context.Context, cfg *config.Config) error {
	logger := utils.GetLogger()
	logger.Info("开始集群巡检...")

	// 初始化存储
	var storageManager *storage.StorageManager
	if cfg.Storage.Enabled {
		storageCfg := &storage.StorageConfig{
			Driver:   cfg.Storage.Driver,
			DSN:      cfg.Storage.DSN,
			MaxConns: cfg.Storage.MaxConns,
		}
		var err error
		storageManager, err = storage.NewStorageManager(storageCfg)
		if err != nil {
			logger.Warnw("初始化存储失败，将不保存历史数据", "error", err)
		} else {
			defer storageManager.Close()
		}
	}

	// 初始化告警管理器
	alertManager := alerting.NewAlertManager(&cfg.Alerting)

	// 注册告警发送器
	if cfg.Alerting.Slack.Enabled {
		slackSender := alerting.NewSlackSender(
			cfg.Alerting.Slack.Webhook,
			cfg.Alerting.Slack.Channel,
			"K8s巡检机器人",
		)
		alertManager.RegisterSender(slackSender)
	}

	if cfg.Alerting.Webhook.Enabled {
		webhookSender := alerting.NewWebhookSender(
			cfg.Alerting.Webhook.URL,
			cfg.Alerting.Webhook.Headers,
		)
		alertManager.RegisterSender(webhookSender)
	}

	// 初始化K8s客户端
	client, err := k8s.NewClient(
		cfg.Inspection.Clusters[0].Kubeconfig,
		cfg.Inspection.Clusters[0].Context,
	)
	if err != nil {
		return fmt.Errorf("创建K8s客户端失败: %v", err)
	}

	// 健康检查
	if err := client.HealthCheck(ctx); err != nil {
		return fmt.Errorf("集群健康检查失败: %v", err)
	}

	// 创建巡检引擎
	engine := analyzer.NewInspectionEngine(&cfg.Inspection, alertManager)

	// 运行巡检
	result, err := engine.Run(ctx, client, cfg.Inspection.Clusters[0].Name)
	if err != nil {
		return fmt.Errorf("巡检执行失败: %v", err)
	}

	// 保存到存储
	if storageManager != nil {
		if err := storageManager.SaveInspectionResult(ctx, result); err != nil {
			logger.Warnw("保存巡检结果失败", "error", err)
		}
	}

	// 生成报告
	reporters := []reporter.Reporter{}

	for _, format := range cfg.Inspection.Reporting.Formats {
		switch format {
		case "console":
			reporters = append(reporters, reporter.NewConsoleReporter())
		case "json":
			reporters = append(reporters,
				reporter.NewJSONReporter(cfg.Inspection.Reporting.OutputDir))
		case "html":
			htmlReporter := reporter.NewHTMLReporter(
				cfg.Inspection.Reporting.OutputDir,
				"./web/templates",
				storageManager,
			)
			reporters = append(reporters, htmlReporter)
		}
	}

	for _, reporter := range reporters {
		if err := reporter.Generate(result); err != nil {
			logger.Errorw("生成报告失败",
				"error", err)
		}
	}

	logger.Infow("巡检完成",
		"cluster", result.ClusterName,
		"score", result.Summary.Score,
		"duration", result.Duration)

	return nil
}

func runServer(ctx context.Context, cfg *config.Config) error {
	logger := utils.GetLogger()
	logger.Info("启动巡检服务...")

	// 初始化存储
	var storageManager *storage.StorageManager
	if cfg.Storage.Enabled {
		storageCfg := &storage.StorageConfig{
			Driver:   cfg.Storage.Driver,
			DSN:      cfg.Storage.DSN,
			MaxConns: cfg.Storage.MaxConns,
		}
		var err error
		storageManager, err = storage.NewStorageManager(storageCfg)
		if err != nil {
			logger.Warnw("初始化存储失败", "error", err)
		} else {
			defer storageManager.Close()
		}
	}

	// 初始化告警管理器
	alertManager := alerting.NewAlertManager(&cfg.Alerting)

	// 注册告警发送器
	if cfg.Alerting.Slack.Enabled {
		slackSender := alerting.NewSlackSender(
			cfg.Alerting.Slack.Webhook,
			cfg.Alerting.Slack.Channel,
			"K8s巡检机器人",
		)
		alertManager.RegisterSender(slackSender)
	}

	if cfg.Alerting.Webhook.Enabled {
		webhookSender := alerting.NewWebhookSender(
			cfg.Alerting.Webhook.URL,
			cfg.Alerting.Webhook.Headers,
		)
		alertManager.RegisterSender(webhookSender)
	}

	// 创建调度器
	scheduler := scheduler.NewScheduler(cfg)

	// 注册巡检回调
	scheduler.RegisterCallback("inspection", func(ctx context.Context, clusterName string) (*types.InspectionResult, error) {
		logger.Infow("执行调度巡检", "cluster", clusterName)

		// 查找集群配置
		var clusterConfig *config.Cluster
		for _, c := range cfg.Inspection.Clusters {
			if c.Name == clusterName {
				clusterConfig = &c
				break
			}
		}

		if clusterConfig == nil {
			return nil, fmt.Errorf("集群配置不存在: %s", clusterName)
		}

		// 创建K8s客户端
		client, err := k8s.NewClient(clusterConfig.Kubeconfig, clusterConfig.Context)
		if err != nil {
			return nil, fmt.Errorf("创建K8s客户端失败: %v", err)
		}

		// 运行巡检
		engine := analyzer.NewInspectionEngine(&cfg.Inspection, alertManager)
		result, err := engine.Run(ctx, client, clusterName)
		if err != nil {
			return nil, err
		}

		// 保存到存储
		if storageManager != nil {
			if err := storageManager.SaveInspectionResult(ctx, result); err != nil {
				logger.Warnw("保存巡检结果失败", "error", err)
			}
		}

		return result, nil
	})

	// 启动调度器
	if err := scheduler.Start(); err != nil {
		return fmt.Errorf("启动调度器失败: %v", err)
	}
	defer scheduler.Stop()

	// 启动API服务器（如果启用）
	if cfg.Server.Enabled {
		apiServer := api.NewServer(scheduler, storageManager, cfg)
		go func() {
			addr := fmt.Sprintf(":%d", cfg.Server.Port)
			logger.Infow("启动API服务器", "addr", addr)
			if err := http.ListenAndServe(addr, apiServer.Handler()); err != nil {
				logger.Errorw("API服务器运行失败", "error", err)
			}
		}()
	}

	logger.Info("巡检服务已启动，按Ctrl+C退出")

	// 等待退出信号
	<-ctx.Done()
	logger.Info("巡检服务已停止")
	return nil
}
