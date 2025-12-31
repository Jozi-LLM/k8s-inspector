package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ym/k8s-inspector/internal/pkg/utils"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	Clientset kubernetes.Interface
	Config    *rest.Config
	Context   string
}

func NewClient(kubeconfig, context string) (*Client, error) {
	var config *rest.Config
	var err error

	// 展开环境变量
	if kubeconfig != "" {
		kubeconfig = os.ExpandEnv(kubeconfig)
	}

	// 如果kubeconfig为空，尝试从默认位置加载
	if kubeconfig == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		if home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	// 创建客户端配置
	if kubeconfig != "" {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
		configOverrides := &clientcmd.ConfigOverrides{}

		// 如果指定了 context，尝试使用它；如果失败，回退到默认 context
		if context != "" {
			configOverrides.CurrentContext = context
			// 先尝试使用指定的 context
			config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				loadingRules, configOverrides).ClientConfig()
			if err != nil {
				// 如果指定的 context 不存在，尝试使用默认 context
				logger := utils.GetGlobalLogger()
				logger.Warnf("指定的 context '%s' 不存在，尝试使用默认 context: %v", context, err)
				configOverrides.CurrentContext = ""
				config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
					loadingRules, configOverrides).ClientConfig()
			}
		} else {
			// 使用默认 context
			config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				loadingRules, configOverrides).ClientConfig()
		}
	} else {
		// 使用in-cluster配置
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("创建k8s客户端配置失败: %v", err)
	}

	// 创建客户端集
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("创建k8s客户端集失败: %v", err)
	}

	logger := utils.GetGlobalLogger()
	logger.Infof("成功连接到K8S集群")

	return &Client{
		Clientset: clientset,
		Config:    config,
		Context:   context,
	}, nil
}

// 获取集群信息
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.Clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("获取K8S集群版本失败: %v", err)
	}
	return nil
}

// 获取集群信息
func (c *Client) GetClusterInfo(ctx context.Context) (map[string]string, error) {
	version, err := c.Clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}

	info := map[string]string{
		"version":  version.String(),
		"platform": version.Platform,
	}

	return info, nil
}
