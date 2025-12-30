// internal/storage/models.go
package storage

import (
	"time"
)

// 巡检结果模型
type InspectionRecord struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)"`
	ClusterName string    `gorm:"index;not null"`
	Timestamp   time.Time `gorm:"index;not null"`
	Duration    float64   `gorm:"not null"` // 秒
	TotalChecks int       `gorm:"not null"`
	Passed      int       `gorm:"not null"`
	Warnings    int       `gorm:"not null"`
	Failed      int       `gorm:"not null"`
	Errors      int       `gorm:"not null"`
	Score       int       `gorm:"not null"`
	RawResult   string    `gorm:"type:text"` // JSON格式的完整结果
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

// 检查详情模型
type CheckDetailRecord struct {
	ID           string `gorm:"primaryKey;type:varchar(36)"`
	InspectionID string `gorm:"index;not null"`
	Category     string `gorm:"index;not null"`
	CheckName    string `gorm:"index;not null"`
	CheckID      string `gorm:"index"`
	Status       string `gorm:"index;not null"` // PASS, WARNING, FAIL, ERROR
	Message      string `gorm:"type:text"`
	Resource     string `gorm:"index"`
	ResourceType string
	Namespace    string    `gorm:"index"`
	Severity     string    `gorm:"index"`     // LOW, MEDIUM, HIGH, CRITICAL
	Evidence     string    `gorm:"type:text"` // JSON格式
	Suggestions  string    `gorm:"type:text"` // JSON格式
	Timestamp    time.Time `gorm:"index;not null"`
}

// 集群统计模型
type ClusterStats struct {
	ClusterName      string    `gorm:"primaryKey;type:varchar(100)"`
	LastInspection   time.Time `gorm:"index"`
	AvgScore         float64   `gorm:"not null;default:0"`
	TotalInspections int       `gorm:"not null;default:0"`
	TotalFailures    int       `gorm:"not null;default:0"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

// 告警记录模型
type AlertRecord struct {
	ID             string    `gorm:"primaryKey;type:varchar(36)"`
	Level          string    `gorm:"index;not null"` // INFO, WARNING, ERROR, CRITICAL
	Title          string    `gorm:"not null"`
	Message        string    `gorm:"type:text"`
	Cluster        string    `gorm:"index;not null"`
	CheckName      string    `gorm:"index"`
	Resource       string    `gorm:"index"`
	Namespace      string    `gorm:"index"`
	Severity       string    `gorm:"index"`
	SentAt         time.Time `gorm:"index;not null"`
	Details        string    `gorm:"type:text"` // JSON格式
	Acknowledged   bool      `gorm:"not null;default:false"`
	AcknowledgedAt *time.Time
	AcknowledgedBy string
}
