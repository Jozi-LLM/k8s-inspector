package checker

import (
	"context"
	"fmt"
	"sync"

	"github.com/ym/k8s-inspector/internal/pkg/k8s"
	"github.com/ym/k8s-inspector/internal/pkg/types"
)

var (
	registry     = make(map[string]types.Checker)
	registryLock sync.RWMutex
)

func Register(checker types.Checker) {
	registryLock.Lock()
	defer registryLock.Unlock()

	name := checker.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("checker %s already registered", name))
	}
}

func Get(name string) (types.Checker, bool) {
	registryLock.RLock()
	defer registryLock.RUnlock()

	checker, exists := registry[name]
	return checker, exists
}

func List() []types.Checker {
	registryLock.RLock()
	defer registryLock.RUnlock()

	checkers := make([]types.Checker, 0, len(registry))
	for _, checker := range registry {
		checkers = append(checkers, checker)
	}
	return checkers
}

func ExecuteAll(ctx context.Context, client *k8s.Client, checkersNames []string) ([]types.CheckDetail, error) {
	return Execute(ctx, client, checkersNames)
}

func Execute(ctx context.Context, client *k8s.Client, checkersNames []string) ([]types.CheckDetail, error) {
	registryLock.RLock()
	defer registryLock.RUnlock()

	var allResults []types.CheckDetail
	for _, name := range checkersNames {
		checker, exists := registry[name]
		if !exists {
			allResults = append(allResults, types.CheckDetail{
				CheckName: name,
				Status:    types.StatusError,
				Message:   fmt.Sprintf("检查器不存在：%s", name),
			})
			continue
		}

		results, err := checker.Execute(ctx, client)
		if err != nil {
			allResults = append(allResults, types.CheckDetail{
				CheckName: name,
				Status:    types.StatusError,
				Message:   fmt.Sprintf("执行检查器失败: %v", err),
			})
			continue
		}
		allResults = append(allResults, results...)
	}
	return allResults, nil
}
