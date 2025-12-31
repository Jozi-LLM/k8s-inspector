// cmd/server/main.go
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/k8s-inspector/internal/api"
	"github.com/ym/k8s-inspector/internal/pkg/config"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
	"github.com/ym/k8s-inspector/internal/scheduler"
)

var (
	configPath = flag.String("config", "./configs/config.yaml", "配置文件路径")
	port       = flag.String("port", "8080", "服务端口")
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	logger := utils.GetLogger()
	defer logger.Sync()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// 创建调度器
	scheduler := scheduler.NewScheduler(cfg)

	// 注册巡检回调
	scheduler.RegisterCallback("inspection", func(ctx context.Context, clusterName string) (*types.InspectionResult, error) {
		logger.Infow("执行调度巡检", "cluster", clusterName)

		// 这里应该调用巡检引擎
		// 为了简化，我们先返回一个模拟结果
		return &types.InspectionResult{
			ClusterName: clusterName,
			Timestamp:   time.Now(),
			Summary: types.Summary{
				TotalChecks: 10,
				Passed:      8,
				Warnings:    1,
				Failed:      1,
				Score:       85,
			},
		}, nil
	})

	// 启动调度器
	if err := scheduler.Start(); err != nil {
		logger.Fatalw("启动调度器失败", "error", err)
	}
	defer scheduler.Stop()

	// 创建API服务器
	apiServer := api.NewServer(scheduler, cfg)

	// 启动HTTP服务器
	server := &http.Server{
		Addr:    ":" + *port,
		Handler: apiServer.Handler(),
	}

	go func() {
		logger.Infow("启动API服务器", "port", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalw("启动HTTP服务器失败", "error", err)
		}
	}()

	// 等待终止信号
	<-signalCh
	logger.Info("收到终止信号，正在关闭...")

	// 优雅关闭
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Errorw("关闭HTTP服务器失败", "error", err)
	}

	cancel()
	logger.Info("服务已关闭")
}
