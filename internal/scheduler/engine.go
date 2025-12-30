// internal/scheduler/engine.go
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/ym/k8s-inspector/internal/pkg/config"
	"github.com/ym/k8s-inspector/internal/pkg/types"
	"github.com/ym/k8s-inspector/internal/pkg/utils"
)

// 任务状态
type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
	JobStatusCancelled JobStatus = "CANCELLED"
)

// 调度任务
type ScheduledJob struct {
	ID         string
	Name       string
	Schedule   string
	Cluster    string
	Status     JobStatus
	LastRun    *time.Time
	NextRun    *time.Time
	LastResult *types.InspectionResult
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// 调度引擎
type Scheduler struct {
	cron      *cron.Cron
	jobs      map[string]*ScheduledJob
	jobMutex  sync.RWMutex
	callbacks map[string]func(context.Context, string) (*types.InspectionResult, error)
	config    *config.Config
	logger    *utils.Logger
	running   bool
}

// 创建调度器
func NewScheduler(cfg *config.Config) *Scheduler {
	// 使用秒级精度
	c := cron.New(cron.WithSeconds())

	return &Scheduler{
		cron:      c,
		jobs:      make(map[string]*ScheduledJob),
		callbacks: make(map[string]func(context.Context, string) (*types.InspectionResult, error)),
		config:    cfg,
		logger:    utils.GetGlobalLogger(),
		running:   false,
	}
}

// 注册任务回调
func (s *Scheduler) RegisterCallback(jobType string, callback func(context.Context, string) (*types.InspectionResult, error)) {
	s.callbacks[jobType] = callback
}

// 添加调度任务
func (s *Scheduler) AddJob(jobID, schedule, cluster string) (string, error) {
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()

	// 检查是否已存在
	if _, exists := s.jobs[jobID]; exists {
		return "", fmt.Errorf("任务已存在: %s", jobID)
	}

	// 验证cron表达式
	if err := validateCronSchedule(schedule); err != nil {
		return "", fmt.Errorf("无效的调度表达式: %v", err)
	}

	// 创建任务
	job := &ScheduledJob{
		ID:        jobID,
		Name:      fmt.Sprintf("inspection-%s", cluster),
		Schedule:  schedule,
		Cluster:   cluster,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 计算下一次运行时间
	scheduleParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := scheduleParser.Parse(schedule)
	if err != nil {
		return "", err
	}
	nextRun := sched.Next(time.Now())
	job.NextRun = &nextRun

	// 添加到cron调度器
	entryID, err := s.cron.AddFunc(schedule, s.createJobFunc(job))
	if err != nil {
		return "", err
	}

	job.ID = fmt.Sprintf("%s-%d", jobID, entryID)
	s.jobs[job.ID] = job

	s.logger.Infow("添加调度任务",
		"job_id", job.ID,
		"schedule", schedule,
		"cluster", cluster,
		"next_run", job.NextRun)

	return job.ID, nil
}

// 创建任务执行函数
func (s *Scheduler) createJobFunc(job *ScheduledJob) func() {
	return func() {
		s.executeJob(job)
	}
}

// 执行任务
func (s *Scheduler) executeJob(job *ScheduledJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	s.jobMutex.Lock()
	job.Status = JobStatusRunning
	job.LastRun = &[]time.Time{time.Now()}[0]
	job.UpdatedAt = time.Now()
	s.jobMutex.Unlock()

	s.logger.Infow("开始执行调度任务",
		"job_id", job.ID,
		"cluster", job.Cluster)

	// 执行巡检回调
	if callback, exists := s.callbacks["inspection"]; exists {
		result, err := callback(ctx, job.Cluster)

		s.jobMutex.Lock()
		if err != nil {
			job.Status = JobStatusFailed
			s.logger.Errorw("调度任务执行失败",
				"job_id", job.ID,
				"error", err)
		} else {
			job.Status = JobStatusCompleted
			job.LastResult = result
			s.logger.Infow("调度任务执行完成",
				"job_id", job.ID,
				"duration", result.Duration,
				"score", result.Summary.Score)
		}

		// 更新下次运行时间
		scheduleParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		sched, _ := scheduleParser.Parse(job.Schedule)
		nextRun := sched.Next(time.Now())
		job.NextRun = &nextRun
		job.UpdatedAt = time.Now()
		s.jobMutex.Unlock()
	} else {
		s.logger.Errorw("未找到任务回调",
			"job_id", job.ID,
			"job_type", "inspection")
	}
}

// 启动调度器
func (s *Scheduler) Start() error {
	if s.running {
		return fmt.Errorf("调度器已在运行")
	}

	s.cron.Start()
	s.running = true
	s.logger.Info("调度器已启动")

	// 如果配置了定时任务，自动添加
	if s.config.Inspector.Schedule != "" {
		for _, cluster := range s.config.Inspector.Clusters {
			jobID := fmt.Sprintf("auto-%s", cluster.Name)
			if _, err := s.AddJob(jobID, s.config.Inspector.Schedule, cluster.Name); err != nil {
				s.logger.Errorw("自动添加调度任务失败",
					"cluster", cluster.Name,
					"error", err)
			}
		}
	}

	return nil
}

// 停止调度器
func (s *Scheduler) Stop() {
	if s.running {
		s.cron.Stop()
		s.running = false
		s.logger.Info("调度器已停止")
	}
}

// 获取所有任务
func (s *Scheduler) GetJobs() []*ScheduledJob {
	s.jobMutex.RLock()
	defer s.jobMutex.RUnlock()

	jobs := make([]*ScheduledJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	return jobs
}

// 获取任务详情
func (s *Scheduler) GetJob(jobID string) (*ScheduledJob, error) {
	s.jobMutex.RLock()
	defer s.jobMutex.RUnlock()

	job, exists := s.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", jobID)
	}

	return job, nil
}

// 删除任务
func (s *Scheduler) RemoveJob(jobID string) error {
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()

	_, exists := s.jobs[jobID]
	if !exists {
		return fmt.Errorf("任务不存在: %s", jobID)
	}

	// 从cron调度器中移除
	// 注意：cron库没有直接通过ID移除的方法
	// 这里我们停止调度器并重建（简化实现）
	s.cron.Stop()
	delete(s.jobs, jobID)

	// 重建除被删除任务外的所有任务
	newCron := cron.New(cron.WithSeconds())
	for _, j := range s.jobs {
		_, err := newCron.AddFunc(j.Schedule, s.createJobFunc(j))
		if err != nil {
			s.logger.Errorw("重建任务失败", "job_id", j.ID, "error", err)
		}
	}

	s.cron = newCron
	if s.running {
		s.cron.Start()
	}

	s.logger.Infow("删除调度任务", "job_id", jobID)
	return nil
}

// 立即运行任务
func (s *Scheduler) RunJobNow(jobID string) error {
	s.jobMutex.RLock()
	job, exists := s.jobs[jobID]
	s.jobMutex.RUnlock()

	if !exists {
		return fmt.Errorf("任务不存在: %s", jobID)
	}

	// 在goroutine中立即执行
	go s.executeJob(job)
	return nil
}

// 验证cron表达式
func validateCronSchedule(schedule string) error {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(schedule)
	return err
}
