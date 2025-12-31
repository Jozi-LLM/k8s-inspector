// internal/pkg/types/types.go
package types

import (
	"context"
	"time"

	"github.com/ym/k8s-inspector/internal/pkg/k8s"
)

type CheckStatus string

const (
	StatusPass    CheckStatus = "PASS"
	StatusWarning CheckStatus = "WARNING"
	StatusFail    CheckStatus = "FAIL"
	StatusError   CheckStatus = "ERROR"
	StatusSkipped CheckStatus = "SKIPPED"
)

type SeverityLevel string

const (
	SeverityLow      SeverityLevel = "LOW"
	SeverityMedium   SeverityLevel = "MEDIUM"
	SeverityHigh     SeverityLevel = "HIGH"
	SeverityCritical SeverityLevel = "CRITICAL"
)

type CheckDetail struct {
	Category     string        `json:"category"`
	CheckName    string        `json:"check_name"`
	CheckID      string        `json:"check_id"`
	Status       CheckStatus   `json:"status"`
	Message      string        `json:"message"`
	Resource     string        `json:"resource"`
	ResourceType string        `json:"resource_type"`
	Namespace    string        `json:"namespace"`
	Severity     SeverityLevel `json:"severity"`
	Evidence     interface{}   `json:"evidence"`
	Suggestions  []string      `json:"suggestions"`
	Timestamp    time.Time     `json:"timestamp"`
	Duration     time.Duration `json:"duration"`
}

type Summary struct {
	TotalChecks int `json:"total_checks"`
	Passed      int `json:"passed"`
	Warnings    int `json:"warnings"`
	Failed      int `json:"failed"`
	Errors      int `json:"errors"`
	Skipped     int `json:"skipped"`
	Score       int `json:"score"`
}

type InspectionResult struct {
	ClusterName     string            `json:"cluster_name"`
	ClusterInfo     map[string]string `json:"cluster_info"`
	Timestamp       time.Time         `json:"timestamp"`
	Duration        time.Duration     `json:"duration"`
	Summary         Summary           `json:"summary"`
	Details         []CheckDetail     `json:"details"`
	Recommendations []string          `json:"recommendations"`
	Metadata        Metadata          `json:"metadata"`
}

type Metadata struct {
	ToolVersion string            `json:"tool_version"`
	GeneratedBy string            `json:"generated_by"`
	CustomData  map[string]string `json:"custom_data,omitempty"`
}

type Checker interface {
	Name() string
	Description() string
	Category() string
	RequiredPermissions() []string
	Execute(ctx context.Context, client *k8s.Client) ([]CheckDetail, error)
}

type ScoreHistory struct {
	Date  string `json:"date"`
	Score int    `json:"score"`
}

type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}
