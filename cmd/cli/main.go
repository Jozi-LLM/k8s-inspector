package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/k8s-inspector/internal/analyzer"
	"github.com/ym/k8s-inspector/internal/pkg/config"
	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
	"github.com/ym/k8s-inspector/internal/reports"
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

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		log.Println("收到终止信号，正在关闭...")
		cancel()
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()

	// 根据模式运行
	switch *mode {
	case "inspect":
		if err := runInspection(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	case "serve":
		if err := runServer(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("未知模式: %s", *mode)
	}
}

func runInspection(ctx context.Context, cfg *config.Config) error {
	logger := utils.GetGlobalLogger()
	logger.Info("开始集群巡检...")

	// 初始化K8s客户端
	client, err := k8s.NewClient(
		cfg.Inspector.Clusters[0].Kubeconfig,
		cfg.Inspector.Clusters[0].Context,
	)
	if err != nil {
		return fmt.Errorf("创建K8s客户端失败: %v", err)
	}

	// 健康检查
	if err := client.HealthCheck(ctx); err != nil {
		return fmt.Errorf("集群健康检查失败: %v", err)
	}

	// 创建巡检引擎
	engine := analyzer.NewInspectionEngine(&cfg.Inspector)

	// 运行巡检
	result, err := engine.Run(ctx, client, cfg.Inspector.Clusters[0].Name)
	if err != nil {
		return fmt.Errorf("巡检执行失败: %v", err)
	}

	// 生成报告
	reporters := []reports.Reporter{}
	reportFormats := cfg.Inspector.Reporting.Formats

	for _, format := range reportFormats {
		switch format {
		case "console":
			reporters = append(reporters, reports.NewConsoleReporter())
		case "json":
			reporters = append(reporters,
				reports.NewJSONReporter(cfg.Inspector.Reporting.OutputDir))
		}
	}

	for i, reporter := range reporters {
		if err := reporter.Generate(result); err != nil {
			logger.Errorw("生成报告失败",
				"format", reportFormats[i],
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
	log.Println("启动巡检服务...")
	// 后续实现服务模式
	<-ctx.Done()
	return nil
}
