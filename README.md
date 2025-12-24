# K8s集群自动化巡检工具 - 功能文档与开发思路

## 一、项目概述

### 1.1 项目目标
开发一个基于Go语言的K8s集群自动化巡检工具，用于定期检查集群健康状态、资源使用情况、配置合规性和安全性，并生成结构化巡检报告。

### 1.2 核心价值
- **自动化巡检**：减少人工检查工作量
- **主动预警**：提前发现潜在问题
- **合规检查**：确保集群配置符合最佳实践
- **历史对比**：追踪集群状态变化趋势

## 二、技术架构设计

### 2.1 技术栈选择
- **语言**: Go 1.19+
- **K8s客户端**: client-go
- **配置管理**: Viper
- **日志**: Zap
- **报告生成**: HTML/PDF/JSON
- **数据存储**: SQLite/PostgreSQL（可选）
- **任务调度**: Cron内置调度

### 2.2 架构模式
```
┌─────────────────┐
│   Web Dashboard │  (可选)
└────────┬────────┘
         │
┌────────▼────────┐    ┌──────────────┐
│   REST API      │◄──►│  数据库       │
└────────┬────────┘    └──────────────┘
         │
┌────────▼────────┐    ┌──────────────┐
│ 巡检调度引擎    │───►│ 插件系统      │
└────────┬────────┘    └──────────────┘
         │
┌────────▼────────┐    ┌──────────────┐
│ 检查器集合      │◄──►│ K8s API      │
└─────────────────┘    └──────────────┘
```

## 三、核心功能模块

### 3.1 集群基础状态检查
```go
// 伪代码结构
type BaseChecker struct {
    // 节点状态检查
    - 节点Ready状态
    - 节点资源压力（CPU/Memory/Disk）
    - 节点内核版本
    - 节点内核参数
    
    // 核心组件健康检查
    - API Server可用性
    - Etcd健康状态
    - Controller Manager
    - Scheduler
    - CoreDNS
    - CNI插件状态
}
```

### 3.2 资源使用分析
```go
type ResourceChecker struct {
    // 资源配额检查
    - 命名空间配额使用率
    - 集群资源总体使用率
    - 资源请求/限制合规性
    
    // Pod资源分析
    - CPU/Memory使用率（对比Request/Limit）
    - 僵尸Pod检测
    - 频繁重启Pod
    - 镜像拉取失败
}
```

### 3.3 工作负载健康度
```go
type WorkloadChecker struct {
    // Deployment检查
    - 副本数可用性
    - 更新策略合规
    - 滚动更新状态
    
    // StatefulSet检查
    - 有序启动/删除
    - PVC绑定状态
    
    // DaemonSet检查
    - 节点覆盖率
    - 更新策略
}
```

### 3.4 网络与存储检查
```go
type NetworkStorageChecker struct {
    // 网络检查
    - Service端点可用性
    - Ingress配置状态
    - NetworkPolicy合规性
    - 网络连通性测试
    
    // 存储检查
    - PVC/PV状态
    - StorageClass配置
    - 卷使用率预警
}
```

### 3.5 安全合规检查
```go
type SecurityChecker struct {
    // RBAC检查
    - 过宽权限检测
    - ServiceAccount安全
    - 集群角色绑定审计
    
    // 安全配置
    - Pod安全策略
    - 安全上下文检查
    - 镜像来源验证
    
    // 证书检查
    - 证书过期时间
    - 证书轮换状态
}
```

### 3.6 配置与最佳实践
```go
type BestPracticeChecker struct {
    // 标签/注解检查
    - 必需标签缺失
    - 注解合规性
    
    // 资源配置
    - HPA配置合理性
    - PDB存在性检查
    - 资源Quality of Service
    
    // 监控与日志
    - 监控注解检查
    - 日志采集配置
}
```

## 四、详细设计

### 4.1 项目结构
```
k8s-inspector/
├── cmd/
│   ├── cli/           # 命令行入口
│   └── server/        # Web服务入口
├── internal/
│   ├── checker/       # 检查器实现
│   │   ├── base/
│   │   ├── resource/
│   │   ├── security/
│   │   └── network/
│   ├── collector/     # 数据收集器
│   ├── analyzer/      # 分析引擎
│   ├── reporter/      # 报告生成器
│   ├── scheduler/     # 任务调度
│   └── api/           # REST API
├── pkg/
│   ├── k8s/           # K8s客户端封装
│   ├── config/        # 配置管理
│   └── utils/         # 工具函数
├── configs/           # 配置文件
├── reports/           # 报告输出目录
└── web/               # Web前端(可选)
```

### 4.2 配置文件示例（YAML）
```yaml
# config/config.yaml
inspection:
  schedule: "0 */6 * * *"  # 每6小时执行
  clusters:
    - name: "production"
      kubeconfig: "/path/to/kubeconfig"
      context: "prod-cluster"
    
  checks:
    enabled:
      - node_health
      - resource_usage
      - security_compliance
      - best_practices
    
    thresholds:
      cpu_usage_warning: 80
      memory_usage_warning: 85
      pod_restart_warning: 5
  
  reporting:
    formats: ["html", "json", "console"]
    output_dir: "./reports"
    email:
      enabled: false
      smtp_server: "smtp.example.com"
      recipients: ["admin@example.com"]
  
  alerting:
    slack:
      enabled: true
      webhook: "https://hooks.slack.com/..."
    webhook:
      enabled: false
      url: "http://alert-server/webhook"
```

### 4.3 核心数据结构
```go
// 检查结果结构
type InspectionResult struct {
    ClusterName    string         `json:"cluster_name"`
    Timestamp      time.Time      `json:"timestamp"`
    Duration       time.Duration  `json:"duration"`
    Summary        Summary        `json:"summary"`
    Details        []CheckDetail  `json:"details"`
    Recommendations []string      `json:"recommendations"`
}

type CheckDetail struct {
    Category     string        `json:"category"`
    CheckName    string        `json:"check_name"`
    Status       CheckStatus   `json:"status"`  // PASS, WARNING, FAIL
    Message      string        `json:"message"`
    Resource     string        `json:"resource"`
    Namespace    string        `json:"namespace"`
    Severity     SeverityLevel `json:"severity"`
    Evidence     interface{}   `json:"evidence"`
}

type Summary struct {
    TotalChecks  int `json:"total_checks"`
    Passed       int `json:"passed"`
    Warnings     int `json:"warnings"`
    Failed       int `json:"failed"`
    Score        int `json:"score"`  // 健康评分(0-100)
}
```

### 4.4 插件系统设计
```go
// 检查器接口
type Checker interface {
    Name() string
    Description() string
    Execute(ctx context.Context, client kubernetes.Interface) ([]CheckDetail, error)
    RequiredPermissions() []string
}

// 注册机制
var checkers = make(map[string]Checker)

func RegisterChecker(name string, checker Checker) {
    checkers[name] = checker
}

// 插件加载
func LoadPlugins(pluginDir string) error {
    // 动态加载.so文件或配置驱动
}
```

## 五、开发计划

### Phase 1: 基础框架（2-3周）
1. 项目初始化与基础结构
2. K8s客户端集成与认证
3. 配置管理模块
4. 基础检查器实现（节点、Pod状态）

### Phase 2: 核心功能（3-4周）
1. 资源使用分析模块
2. 安全合规检查器
3. 报告生成引擎（HTML/JSON）
4. 命令行界面

### Phase 3: 高级功能（2-3周）
1. 调度系统与定时任务
2. 插件系统
3. 数据持久化（SQLite）
4. 告警集成（Slack/Webhook）

### Phase 4: 扩展功能（2-3周）
1. REST API服务
2. Web仪表板（可选）
3. 历史数据对比
4. 性能优化

## 六、输出物设计

### 6.1 HTML报告
- 集群概览仪表板
- 问题分类展示
- 趋势图表（使用Chart.js）
- 可交互的详细信息
- 导出功能（PDF/CSV）

### 6.2 JSON报告
```json
{
  "metadata": {
    "cluster": "production",
    "timestamp": "2024-01-15T10:30:00Z",
    "version": "1.0.0"
  },
  "summary": {
    "score": 85,
    "passed": 42,
    "warnings": 5,
    "failed": 2
  },
  "details": [
    {
      "category": "security",
      "check": "privileged_containers",
      "status": "FAIL",
      "severity": "high",
      "message": "发现特权容器运行"
    }
  ]
}
```

### 6.3 控制台输出
```
╔════════════════════════════════════════════════════════════╗
║                   K8s集群巡检报告                           ║
╠════════════════════════════════════════════════════════════╣
║ 集群: production                                           ║
║ 时间: 2024-01-15 10:30:00                                 ║
║ 健康分: 85/100                                            ║
╠════════════════════════════════════════════════════════════╣
║ 检查项        总数    通过    警告    失败                 ║
║ 节点健康       12     10      1       1                   ║
║ 资源使用       15     14      1       0                   ║
║ 安全合规       20     18      2       0                   ║
╚════════════════════════════════════════════════════════════╝

[高危] 节点 worker-03 磁盘压力过高 (95%)
[警告] default命名空间缺少资源限制
[通过] 所有核心组件运行正常
```

## 七、部署与使用

### 7.1 部署方式
1. **二进制部署**: 下载编译好的二进制文件
2. **容器化部署**: Docker镜像
3. **K8s Job/CronJob**: 集群内运行
4. **Sidecar模式**: 与应用一同部署

### 7.2 使用示例
```bash
# 单次巡检
./k8s-inspector inspect --config config.yaml

# 定时运行
./k8s-inspector serve --schedule "0 */6 * * *"

# 指定检查项
./k8s-inspector inspect --checks security,resources

# 生成HTML报告
./k8s-inspector inspect --format html --output report.html
```

### 7.3 K8s CronJob示例
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8s-inspector
spec:
  schedule: "0 */6 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: inspector-sa
          containers:
          - name: inspector
            image: k8s-inspector:latest
            args: ["inspect", "--config", "/config/config.yaml"]
            volumeMounts:
            - name: config
              mountPath: /config
          volumes:
          - name: config
            configMap:
              name: inspector-config
```

## 八、安全考虑

1. **最小权限原则**: 创建专用的ServiceAccount
2. **RBAC配置**: 精确控制访问权限
3. **敏感信息**: 加密存储凭据
4. **审计日志**: 记录所有操作
5. **网络隔离**: 限制网络访问范围

## 九、监控与维护

1. **工具自监控**: 监控巡检工具自身状态
2. **版本管理**: 定期更新client-go版本
3. **检查项更新**: 根据K8s版本更新检查规则
4. **性能优化**: 大数据量集群的优化策略

## 十、扩展方向

1. **多云支持**: 同时巡检多个云厂商的K8s集群
2. **自定义检查**: 用户自定义检查规则
3. **机器学习**: 异常检测与智能预警
4. **集成生态**: 与Prometheus、Grafana等集成
5. **GitOps集成**: 与ArgoCD/Flux的配置合规检查

---

这个方案提供了完整的开发框架，您可以根据实际需求调整检查项和优先级。建议从基础框架开始，逐步迭代完善功能。