# Simple ConfigMap-to-Secret Controller

一个简单的 Kubernetes Controller 示例，演示 Controller 的核心模式。

## 功能

监听带有 `simple-controller/sync-to-secret` annotation 的 ConfigMap，自动将其数据同步到同名 Secret。

## 运行步骤

### 1. 确保有可用的 Kubernetes 集群

```powershell
# 检查集群连接
kubectl cluster-info

# 如果没有集群，可以使用 Docker Desktop 的 Kubernetes 
# 或者安装 minikube/kind
```

### 2. 下载依赖

```powershell
cd D:\workspace\k8s-demo\simple-controller
go mod tidy
```

### 3. 运行 Controller

```powershell
# 监听所有 namespace
go run main.go

# 或只监听 default namespace
go run main.go -namespace=default
```

### 4. 测试

打开另一个终端：

```powershell
# 应用测试 ConfigMap
kubectl apply -f test-configmap.yaml

# 查看创建的 Secret
kubectl get secret my-app-config-synced -o yaml

# 修改 ConfigMap，观察 Secret 是否同步更新
kubectl patch configmap my-app-config -p '{"data":{"NEW_KEY":"new-value"}}'

# 删除 ConfigMap，Secret 也会被自动删除（因为 OwnerReference）
kubectl delete configmap my-app-config
kubectl get secret my-app-config-synced  # 应该也被删除了
```

## 代码解析

### 核心流程

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────────┐
│  ConfigMap  │────▶│  Reconcile  │────▶│  Create/Update      │
│  (watched)  │     │  Function   │     │  Secret             │
└─────────────┘     └─────────────┘     └─────────────────────┘
```

### 关键点

1. **Watch**: `ctrl.NewControllerManagedBy(mgr).For(&corev1.ConfigMap{})` 监听 ConfigMap 变化
2. **Filter**: 通过检查 annotation 来决定是否处理
3. **Reconcile**: 将期望状态（ConfigMap 数据）调谐到 Secret
4. **OwnerReference**: 设置所有权关系，实现级联删除

## 扩展练习

1. 添加 Finalizer，在删除时执行额外清理
2. 添加 Status 字段记录同步状态
3. 使用 CRD 代替 annotation 方式
4. 添加 Webhook 进行数据验证
