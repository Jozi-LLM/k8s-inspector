package reporter

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

// HTML报告生成器
type HTMLReporter struct {
	outputDir   string
	templateDir string
	storage     StorageInterface
}

// 存储接口（用于获取历史数据）
type StorageInterface interface {
	GetScoreHistory(cluster string, days int) ([]ScoreHistory, error)
	GetCategoryDistribution(cluster string) ([]CategoryCount, error)
}

// 历史分数数据
type ScoreHistory struct {
	Date  string `json:"date"`
	Score int    `json:"score"`
}

// 分类统计
type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// 模板数据
type HTMLTemplateData struct {
	*types.InspectionResult
	CategoryCounts       map[string]int
	CategoryDistribution []CategoryCount
	ScoreHistory         []ScoreHistory
}

// 创建HTML报告生成器
func NewHTMLReporter(outputDir, templateDir string, storage StorageInterface) *HTMLReporter {
	if templateDir == "" {
		templateDir = "./web/templates"
	}

	return &HTMLReporter{
		outputDir:   outputDir,
		templateDir: templateDir,
		storage:     storage,
	}
}

func (r *HTMLReporter) Generate(result *types.InspectionResult) error {
	logger := utils.GetLogger()

	// 确保输出目录存在
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 加载模板
	tmpl, err := r.loadTemplate()
	if err != nil {
		return fmt.Errorf("加载模板失败: %v", err)
	}

	// 准备模板数据
	data, err := r.prepareTemplateData(result)
	if err != nil {
		return fmt.Errorf("准备模板数据失败: %v", err)
	}

	// 生成文件名
	timestamp := result.Timestamp.Format("20060102-150405")
	filename := fmt.Sprintf("inspection-%s-%s.html", result.ClusterName, timestamp)
	filepath := filepath.Join(r.outputDir, filename)

	// 创建输出文件
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 执行模板
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("执行模板失败: %v", err)
	}

	// 创建最新的报告链接
	latestPath := filepath.Join(r.outputDir, fmt.Sprintf("latest-%s.html", result.ClusterName))
	if err := os.Remove(latestPath); err != nil && !os.IsNotExist(err) {
		logger.Warnw("删除旧的最新报告失败", "error", err)
	}

	if err := os.Symlink(filename, latestPath); err != nil {
		// 如果创建符号链接失败，复制文件
		if copyErr := copyFile(filepath, latestPath); copyErr != nil {
			logger.Warnw("创建最新报告链接失败", "error", copyErr)
		}
	}

	logger.Infow("HTML报告已生成",
		"file", filepath,
		"latest", latestPath)

	return nil
}

// 加载模板
func (r *HTMLReporter) loadTemplate() (*template.Template, error) {
	templatePath := filepath.Join(r.templateDir, "report.html")

	// 检查模板文件是否存在
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		// 如果模板文件不存在，使用内嵌模板
		return r.createDefaultTemplate()
	}

	// 加载模板文件
	tmpl, err := template.New("report.html").
		Funcs(template.FuncMap{
			"toLower": strings.ToLower,
			"toJson":  toJSON,
			"add":     func(a, b int) int { return a + b },
		}).
		ParseFiles(templatePath)

	if err != nil {
		return nil, fmt.Errorf("解析模板文件失败: %v", err)
	}

	return tmpl, nil
}

// 创建默认模板
func (r *HTMLReporter) createDefaultTemplate() (*template.Template, error) {
	// 这里可以嵌入一个基本的HTML模板
	// 为了简化，我们返回错误
	return nil, fmt.Errorf("模板文件不存在: %s", filepath.Join(r.templateDir, "report.html"))
}

// 准备模板数据
func (r *HTMLReporter) prepareTemplateData(result *types.InspectionResult) (*HTMLTemplateData, error) {
	data := &HTMLTemplateData{
		InspectionResult: result,
		CategoryCounts:   make(map[string]int),
	}

	// 计算分类统计
	for _, detail := range result.Details {
		data.CategoryCounts[detail.Category]++
	}

	// 转换为分类分布数据
	for category, count := range data.CategoryCounts {
		data.CategoryDistribution = append(data.CategoryDistribution, CategoryCount{
			Category: category,
			Count:    count,
		})
	}

	// 按分类名称排序
	sort.Slice(data.CategoryDistribution, func(i, j int) bool {
		return data.CategoryDistribution[i].Category < data.CategoryDistribution[j].Category
	})

	// 获取历史数据
	if r.storage != nil {
		// 获取最近7天的健康分历史
		if history, err := r.storage.GetScoreHistory(result.ClusterName, 7); err == nil {
			data.ScoreHistory = history
		} else {
			// 使用当前数据填充
			data.ScoreHistory = []ScoreHistory{
				{
					Date:  result.Timestamp.Format("2006-01-02"),
					Score: result.Summary.Score,
				},
			}
		}
	} else {
		// 没有存储，使用当前数据
		data.ScoreHistory = []ScoreHistory{
			{
				Date:  result.Timestamp.Format("2006-01-02"),
				Score: result.Summary.Score,
			},
		}
	}

	return data, nil
}

// 转换为JSON字符串（用于模板函数）
func toJSON(v interface{}) (template.JS, error) {
	// 这里应该实现JSON序列化
	// 为了简化，返回空JSON
	return template.JS("{}"), nil
}

// 复制文件
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}
