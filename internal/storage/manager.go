// internal/storage/manager.go
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 存储管理器
type StorageManager struct {
	db     *gorm.DB
	config *StorageConfig
	logger *utils.Logger
}

// 存储配置
type StorageConfig struct {
	Driver   string // sqlite, mysql, postgres
	DSN      string // 连接字符串
	MaxConns int    // 最大连接数
}

// 创建存储管理器
func NewStorageManager(cfg *StorageConfig) (*StorageManager, error) {
	logger := utils.GetGlobalLogger()

	if cfg.Driver == "" {
		cfg.Driver = "sqlite"
	}

	if cfg.DSN == "" && cfg.Driver == "sqlite" {
		// 默认SQLite数据库路径
		cfg.DSN = "./data/k8s-inspector.db"
	}

	// 确保目录存在
	if cfg.Driver == "sqlite" {
		dir := filepath.Dir(cfg.DSN)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %v", err)
		}
	}

	var db *gorm.DB
	var err error

	switch cfg.Driver {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(cfg.DSN), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		})
	case "mysql":
		// 需要导入 gorm.io/driver/mysql
		// db, err = gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{})
		return nil, fmt.Errorf("MySQL暂不支持")
	case "postgres":
		// 需要导入 gorm.io/driver/postgres
		// db, err = gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
		return nil, fmt.Errorf("PostgreSQL暂不支持")
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}

	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %v", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %v", err)
	}

	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(cfg.MaxConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	manager := &StorageManager{
		db:     db,
		config: cfg,
		logger: logger,
	}

	// 自动迁移表结构
	if err := manager.migrate(); err != nil {
		return nil, err
	}

	logger.Infow("存储管理器初始化完成",
		"driver", cfg.Driver,
		"dsn", cfg.DSN)

	return manager, nil
}

// 自动迁移表结构
func (m *StorageManager) migrate() error {
	return m.db.AutoMigrate(
		&InspectionRecord{},
		&CheckDetailRecord{},
		&ClusterStats{},
		&AlertRecord{},
	)
}

// 保存巡检结果
func (m *StorageManager) SaveInspectionResult(ctx context.Context, result *types.InspectionResult) error {
	// 将完整结果转为JSON
	rawResult, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化巡检结果失败: %v", err)
	}

	// 创建巡检记录
	record := &InspectionRecord{
		ID:          generateID(),
		ClusterName: result.ClusterName,
		Timestamp:   result.Timestamp,
		Duration:    result.Duration.Seconds(),
		TotalChecks: result.Summary.TotalChecks,
		Passed:      result.Summary.Passed,
		Warnings:    result.Summary.Warnings,
		Failed:      result.Summary.Failed,
		Errors:      result.Summary.Errors,
		Score:       result.Summary.Score,
		RawResult:   string(rawResult),
		CreatedAt:   time.Now(),
	}

	// 保存到数据库
	if err := m.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("保存巡检记录失败: %v", err)
	}

	// 保存检查详情
	var detailRecords []*CheckDetailRecord
	for _, detail := range result.Details {
		evidenceJSON, _ := json.Marshal(detail.Evidence)
		suggestionsJSON, _ := json.Marshal(detail.Suggestions)

		detailRecord := &CheckDetailRecord{
			ID:           generateID(),
			InspectionID: record.ID,
			Category:     detail.Category,
			CheckName:    detail.CheckName,
			CheckID:      detail.CheckID,
			Status:       string(detail.Status),
			Message:      detail.Message,
			Resource:     detail.Resource,
			ResourceType: detail.ResourceType,
			Namespace:    detail.Namespace,
			Severity:     string(detail.Severity),
			Evidence:     string(evidenceJSON),
			Suggestions:  string(suggestionsJSON),
			Timestamp:    detail.Timestamp,
		}
		detailRecords = append(detailRecords, detailRecord)
	}

	if len(detailRecords) > 0 {
		if err := m.db.WithContext(ctx).CreateInBatches(detailRecords, 100).Error; err != nil {
			m.logger.Warnw("保存检查详情失败", "error", err)
		}
	}

	// 更新集群统计
	m.updateClusterStats(ctx, result)

	m.logger.Infow("巡检结果已保存",
		"cluster", result.ClusterName,
		"score", result.Summary.Score,
		"record_id", record.ID)

	return nil
}

// 更新集群统计
func (m *StorageManager) updateClusterStats(ctx context.Context, result *types.InspectionResult) error {
	var stats ClusterStats

	// 查找现有统计
	m.db.WithContext(ctx).First(&stats, "cluster_name = ?", result.ClusterName)

	if stats.ClusterName == "" {
		// 创建新统计
		stats = ClusterStats{
			ClusterName:      result.ClusterName,
			LastInspection:   result.Timestamp,
			AvgScore:         float64(result.Summary.Score),
			TotalInspections: 1,
			TotalFailures:    result.Summary.Failed + result.Summary.Errors,
			UpdatedAt:        time.Now(),
		}
		return m.db.WithContext(ctx).Create(&stats).Error
	}

	// 更新现有统计
	oldTotal := stats.TotalInspections
	newTotal := oldTotal + 1

	// 计算新的平均分
	stats.AvgScore = (stats.AvgScore*float64(oldTotal) + float64(result.Summary.Score)) / float64(newTotal)
	stats.TotalInspections = newTotal
	stats.TotalFailures += result.Summary.Failed + result.Summary.Errors
	stats.LastInspection = result.Timestamp
	stats.UpdatedAt = time.Now()

	return m.db.WithContext(ctx).Save(&stats).Error
}

// AlertMessage 告警消息结构
type AlertMessage struct {
	ID        string
	Level     string
	Title     string
	Message   string
	Cluster   string
	CheckName string
	Resource  string
	Namespace string
	Severity  string
	Timestamp time.Time
	Details   map[string]interface{}
}

// 保存告警记录
func (m *StorageManager) SaveAlert(ctx context.Context, alert *AlertMessage) error {
	detailsJSON, err := json.Marshal(alert.Details)
	if err != nil {
		return err
	}

	record := &AlertRecord{
		ID:        alert.ID,
		Level:     alert.Level,
		Title:     alert.Title,
		Message:   alert.Message,
		Cluster:   alert.Cluster,
		CheckName: alert.CheckName,
		Resource:  alert.Resource,
		Namespace: alert.Namespace,
		Severity:  alert.Severity,
		SentAt:    alert.Timestamp,
		Details:   string(detailsJSON),
	}

	return m.db.WithContext(ctx).Create(record).Error
}

// 获取历史巡检记录
func (m *StorageManager) GetInspectionHistory(ctx context.Context, cluster string, limit, offset int) ([]*InspectionRecord, error) {
	var records []*InspectionRecord

	query := m.db.WithContext(ctx).Model(&InspectionRecord{})
	if cluster != "" {
		query = query.Where("cluster_name = ?", cluster)
	}

	err := query.Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error

	return records, err
}

// 获取巡检详情
func (m *StorageManager) GetInspectionDetails(ctx context.Context, inspectionID string) (*types.InspectionResult, error) {
	var record InspectionRecord
	if err := m.db.WithContext(ctx).First(&record, "id = ?", inspectionID).Error; err != nil {
		return nil, err
	}

	var result types.InspectionResult
	if err := json.Unmarshal([]byte(record.RawResult), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// 获取检查趋势
func (m *StorageManager) GetCheckTrend(ctx context.Context, cluster, checkName string, days int) ([]TrendData, error) {
	var trend []TrendData

	query := `
		SELECT 
			DATE(timestamp) as date,
			COUNT(*) as total,
			SUM(CASE WHEN status = 'PASS' THEN 1 ELSE 0 END) as passed,
			SUM(CASE WHEN status = 'FAIL' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN status = 'WARNING' THEN 1 ELSE 0 END) as warnings
		FROM check_detail_records
		WHERE check_name = ? 
			AND cluster_name = ?
			AND timestamp >= DATE('now', ? || ' days')
		GROUP BY DATE(timestamp)
		ORDER BY date
	`

	daysParam := fmt.Sprintf("-%d", days)
	err := m.db.WithContext(ctx).Raw(query, checkName, cluster, daysParam).Scan(&trend).Error

	return trend, err
}

// 获取集群统计
func (m *StorageManager) GetClusterStats(ctx context.Context, cluster string) (*ClusterStats, error) {
	var stats ClusterStats
	err := m.db.WithContext(ctx).First(&stats, "cluster_name = ?", cluster).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &stats, err
}

// 关闭数据库连接
func (m *StorageManager) Close() error {
	sqlDB, err := m.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// 生成唯一ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// 趋势数据结构
type TrendData struct {
	Date     string  `json:"date"`
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Warnings int     `json:"warnings"`
	PassRate float64 `json:"pass_rate" gorm:"-"`
}
