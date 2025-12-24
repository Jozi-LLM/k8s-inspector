package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

type JSONReporter struct {
	outputDir string
}

func NewJSONReporter(outputDir string) *JSONReporter {
	return &JSONReporter{
		outputDir: outputDir,
	}
}

func (r *JSONReporter) Generate(result *types.InspectionResult) error {
	logger := utils.GetGlobalLogger()

	// 确保目录存在
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}
	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("inspection_result_%s.json", timestamp)
	filepath := filepath.Join(r.outputDir, filename)

	// 转化json格式
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化JSON失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("写入JSON文件失败: %w", err)
	}
	logger.Infow("JSON报告生成成功", "filepath", filepath)
	return nil

}
