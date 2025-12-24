package utils

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Logger struct {
	*zap.SugaredLogger
	config *LogConfig
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
	File   string `yaml:"file"`
}

func NewLogger(config *LogConfig) (*Logger, error) {
	var zapConfig zap.Config

	if config.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	//设置日志级别
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(config.Level)); err != nil {
		return nil, err
	}
	zapConfig.Level = zap.NewAtomicLevelAt(level)

	//设置日志输出
	var writers []zapcore.WriteSyncer
	if config.Output == "stdout" || config.Output == "both" {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}
	if config.Output == "file" || config.Output == "both" {
		writer := &lumberjack.Logger{
			Filename:   config.File,
			MaxSize:    100, //MB
			MaxBackups: 3,
			MaxAge:     28,
			Compress:   true,
		}
		writers = append(writers, zapcore.AddSync(writer))
	}
	if len(writers) == 0 {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapConfig.EncoderConfig),
		zapcore.NewMultiWriteSyncer(writers...),
		zapConfig.Level,
	)

	logger := zap.New(core, zap.AddCaller())
	sugar := logger.Sugar()

	return &Logger{
		SugaredLogger: sugar,
		config:        config,
	}, nil
}

// 全局日志示例
var globalLogger *Logger

func InitGlobalLogger(config *LogConfig) error {
	logger, err := NewLogger(config)
	if err != nil {
		return err
	}
	globalLogger = logger
	return nil
}

func GetGlobalLogger() *Logger {
	if globalLogger == nil {
		// 使用默认配置创建logger
		config := &LogConfig{
			Level:  "info",
			Format: "console",
			Output: "stdout",
		}
		logger, _ := NewLogger(config)
		globalLogger = logger
	}
	return globalLogger
}
