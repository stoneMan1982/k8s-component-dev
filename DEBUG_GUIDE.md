# Kubernetes Controller 调试指南

## 1. 日志调试

### 基本用法
```go
logger := log.FromContext(ctx)

// Info 级别
logger.Info("Processing resource", "name", obj.Name, "namespace", obj.Namespace)

// Debug 级别 (V(1) 及以上)
logger.V(1).Info("Debug details", "spec", obj.Spec)

// Error 级别
logger.Error(err, "Failed to create resource", "name", obj.Name)
```

### 运行时调整日志级别
```powershell
# 默认级别 (只显示 Info 和 Error)
go run main.go

# 显示 Debug 日志 (V(1))
go run main.go -zap-log-level=debug

# 显示更详细日志 (V(2))
go run main.go -zap-log-level=2

# 开发模式 (彩色输出 + 堆栈跟踪)
go run main.go -zap-devel=true
```

## 2. Delve 调试器

### 安装 Delve
```powershell
go install github.com/go-delve/delve/cmd/dlv@latest
```

### 命令行调试
```powershell
# 启动调试
dlv debug .

# 常用命令
(dlv) break main.go:50          # 设置断点
(dlv) break Reconcile           # 在函数处设断点
(dlv) continue                  # 继续执行
(dlv) next                      # 单步执行
(dlv) step                      # 进入函数
(dlv) print configMap           # 打印变量
(dlv) locals                    # 显示所有局部变量
(dlv) quit                      # 退出
```

### VS Code 调试配置

创建 `.vscode/launch.json`:
```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug Controller",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/main.go",
            "args": ["-namespace=default"],
            "env": {
                "KUBECONFIG": "${env:USERPROFILE}/.kube/config"
            }
        }
    ]
}
```

然后按 F5 启动调试，可以设置断点、查看变量。

## 3. 使用 Events 记录

Events 会显示在 `kubectl describe` 输出中，方便排查问题：

```go
import "k8s.io/client-go/tools/record"

type ConfigMapReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder  // 添加这个字段
}

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 成功事件
    r.Recorder.Event(configMap, corev1.EventTypeNormal, "Synced", "Secret created successfully")
    
    // 警告事件
    r.Recorder.Event(configMap, corev1.EventTypeWarning, "SyncFailed", err.Error())
    
    // 带格式化的事件
    r.Recorder.Eventf(configMap, corev1.EventTypeNormal, "Synced", "Created secret %s", secretName)
}

// 在 main.go 中初始化
reconciler := &ConfigMapReconciler{
    Client:   mgr.GetClient(),
    Scheme:   mgr.GetScheme(),
    Recorder: mgr.GetEventRecorderFor("simple-controller"),
}
```

查看 Events:
```powershell
kubectl describe configmap my-app-config
# 在输出底部会看到 Events 部分
```

## 4. 检查 K8s 资源状态

```powershell
# 查看资源详情
kubectl get configmap my-app-config -o yaml
kubectl get secret my-app-config-synced -o yaml

# 查看 OwnerReference（验证级联删除）
kubectl get secret my-app-config-synced -o jsonpath='{.metadata.ownerReferences}'

# 查看所有相关资源
kubectl get configmap,secret -l app.kubernetes.io/managed-by=simple-controller

# 实时监听变化
kubectl get configmap -w
kubectl get secret -w
```

## 5. 常见 Bug 排查

### 问题：Reconcile 没有触发
```powershell
# 检查 Controller 是否在运行
# 检查是否监听了正确的资源类型和 namespace

# 手动触发（修改资源的 annotation）
kubectl annotate configmap my-app-config debug=true --overwrite
```

### 问题：资源创建失败
```go
// 在代码中打印完整错误
if err := r.Create(ctx, secret); err != nil {
    logger.Error(err, "Create failed", 
        "secret", secret.Name,
        "namespace", secret.Namespace,
        "labels", secret.Labels,
    )
    // 检查 RBAC 权限
    // 检查资源 schema 是否正确
    return ctrl.Result{}, err
}
```

### 问题：权限不足
```powershell
# 查看 Controller 的 ServiceAccount 权限
kubectl auth can-i create secrets --as=system:serviceaccount:default:default

# 本地运行时使用的是你的 kubeconfig，通常有 admin 权限
# 部署到集群时需要配置 RBAC
```

### 问题：无限循环 Reconcile
```go
// 常见原因：每次 Reconcile 都在更新资源
// 解决方案：只在需要时更新

// 错误示范 ❌
r.Update(ctx, secret)  // 总是更新

// 正确示范 ✅
if !reflect.DeepEqual(existingSecret.Data, secret.Data) {
    r.Update(ctx, existingSecret)
}
```

## 6. 实用调试函数

```go
import (
    "encoding/json"
    "fmt"
)

// 打印任意对象为 JSON（调试用）
func debugPrint(name string, obj interface{}) {
    data, _ := json.MarshalIndent(obj, "", "  ")
    fmt.Printf("=== %s ===\n%s\n", name, string(data))
}

// 在 Reconcile 中使用
debugPrint("ConfigMap", configMap)
debugPrint("Secret", secret)
```

## 7. 单元测试调试

```powershell
# 运行测试并显示详细输出
go test -v ./...

# 运行特定测试
go test -v -run TestReconcile ./...

# 带覆盖率
go test -v -cover ./...
```
