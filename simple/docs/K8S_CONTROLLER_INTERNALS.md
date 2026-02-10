# Kubernetes Controller 核心组件详解

本文档详细介绍 Kubernetes Controller 的核心组件及其协作关系。

## 整体架构图

```
                                    Kubernetes API Server
                                            │
                                            │ List & Watch
                                            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Informer                                        │
│  ┌─────────────┐      ┌─────────────┐      ┌─────────────┐                  │
│  │  Reflector  │─────▶│  DeltaFIFO  │─────▶│   Indexer   │                  │
│  │  (生产者)    │      │   (队列)     │      │  (本地缓存)  │                  │
│  └─────────────┘      └──────┬──────┘      └─────────────┘                  │
│                              │                     ▲                         │
│                              │ Pop                 │ Get/List                │
│                              ▼                     │                         │
│                     ┌─────────────┐                │                         │
│                     │  Handler    │                │                         │
│                     │ (事件分发)   │                │                         │
│                     └──────┬──────┘                │                         │
└────────────────────────────┼───────────────────────┼─────────────────────────┘
                             │                       │
                             │ AddFunc/UpdateFunc/   │
                             │ DeleteFunc            │
                             ▼                       │
                     ┌─────────────┐                 │
                     │ Workqueue   │                 │
                     │ (限速队列)   │                 │
                     └──────┬──────┘                 │
                            │                        │
                            │ Get                    │
                            ▼                        │
                     ┌─────────────┐                 │
                     │   Worker    │─────────────────┘
                     │ (Reconcile) │
                     └─────────────┘
```

## 1. Reflector（反射器）

### 作用
Reflector 是 **数据生产者**，负责与 API Server 通信，获取资源的变化事件。

### 工作原理

```go
// Reflector 核心结构
type Reflector struct {
    name          string
    expectedType  reflect.Type       // 监听的资源类型
    store         Store              // 数据存储（DeltaFIFO）
    listerWatcher ListerWatcher      // List-Watch 接口
    resyncPeriod  time.Duration      // 重新同步周期
}
```

**执行流程：**

1. **List 阶段**（启动时）
   - 调用 `listerWatcher.List()` 获取资源的全量数据
   - 将全量数据通过 `Replace()` 写入 DeltaFIFO
   - 记录当前的 `resourceVersion`

2. **Watch 阶段**（持续运行）
   - 调用 `listerWatcher.Watch(resourceVersion)` 建立长连接
   - 监听 API Server 推送的增量事件（ADDED/MODIFIED/DELETED）
   - 将事件写入 DeltaFIFO

3. **重连机制**
   - Watch 连接断开时自动重连
   - 如果 resourceVersion 过期，回退到 List 重新同步

```go
// 简化的工作循环
func (r *Reflector) Run(stopCh <-chan struct{}) {
    // 1. 首次 List
    list, _ := r.listerWatcher.List(metav1.ListOptions{})
    r.store.Replace(list, resourceVersion)
    
    // 2. 持续 Watch
    for {
        watcher, _ := r.listerWatcher.Watch(metav1.ListOptions{
            ResourceVersion: r.lastResourceVersion,
        })
        
        for event := range watcher.ResultChan() {
            switch event.Type {
            case watch.Added:
                r.store.Add(event.Object)
            case watch.Modified:
                r.store.Update(event.Object)
            case watch.Deleted:
                r.store.Delete(event.Object)
            }
        }
    }
}
```

## 2. DeltaFIFO（增量先进先出队列）

### 作用
DeltaFIFO 是连接 Reflector 和 Indexer 的 **缓冲队列**，存储资源的变化事件（Delta）。

### 核心数据结构

```go
type DeltaFIFO struct {
    items map[string]Deltas  // key -> 事件列表
    queue []string           // 有序的 key 队列（FIFO）
}

// Delta 表示一个变化事件
type Delta struct {
    Type   DeltaType  // Added, Updated, Deleted, Sync
    Object interface{}
}

// Deltas 是同一对象的多个事件（会被合并）
type Deltas []Delta
```

### 特点

1. **按 Key 去重**：同一对象的多个事件存储在一起
2. **保序**：按照事件到达顺序处理
3. **事件合并**：连续的 Update 会被合并，减少处理次数

```
示例：同一 Pod 的多次更新

时间线:  Add -> Update -> Update -> Delete

DeltaFIFO 存储:
items["default/my-pod"] = [
    {Type: Added,   Object: pod_v1},
    {Type: Updated, Object: pod_v2},
    {Type: Updated, Object: pod_v3},
    {Type: Deleted, Object: pod_v3},
]
```

### 核心操作

```go
// 生产者调用（Reflector）
func (f *DeltaFIFO) Add(obj interface{}) error
func (f *DeltaFIFO) Update(obj interface{}) error
func (f *DeltaFIFO) Delete(obj interface{}) error

// 消费者调用（Controller）
func (f *DeltaFIFO) Pop(process PopProcessFunc) (interface{}, error)
```

## 3. Indexer（索引器/本地缓存）

### 作用
Indexer 是 **本地缓存**，存储从 API Server 同步的资源对象，并提供索引查询能力。

### 数据结构

```go
type cache struct {
    items   map[string]interface{}  // key -> object
    indexers Indexers               // 索引函数
    indices  Indices                // 索引数据
}

// 索引函数：从对象提取索引值
type IndexFunc func(obj interface{}) ([]string, error)

// 内置索引：按 namespace 索引
func MetaNamespaceIndexFunc(obj interface{}) ([]string, error) {
    meta, _ := meta.Accessor(obj)
    return []string{meta.GetNamespace()}, nil
}
```

### 索引示例

```
对象: Pod{namespace: "default", name: "nginx", labels: {app: "web"}}

存储:
items["default/nginx"] = Pod对象

索引（按 namespace）:
indices["namespace"]["default"] = ["default/nginx", "default/redis", ...]
```

### 常用操作

```go
// 基本 CRUD
indexer.Add(obj)
indexer.Update(obj)
indexer.Delete(obj)
indexer.Get(obj)

// 列表查询
indexer.List()                              // 获取所有对象
indexer.ListKeys()                          // 获取所有 key

// 索引查询（高效）
indexer.ByIndex("namespace", "default")     // 获取 default namespace 的所有对象
indexer.Index("namespace", obj)             // 获取与 obj 同 namespace 的对象
```

## 4. Informer（通知器）

### 作用
Informer 是 **核心协调者**，封装了 Reflector + DeltaFIFO + Indexer，并提供事件回调机制。

### 结构关系

```go
type sharedIndexInformer struct {
    indexer    Indexer              // 本地缓存
    controller Controller           // 包含 Reflector 和 DeltaFIFO
    processor  *sharedProcessor     // 事件处理器（分发给多个 listener）
}
```

### 工作流程

```
1. Reflector 从 API Server 获取事件
2. 事件写入 DeltaFIFO
3. Controller 从 DeltaFIFO Pop 事件
4. 更新 Indexer（本地缓存）
5. 调用注册的 EventHandler（OnAdd/OnUpdate/OnDelete）
```

### 使用方式

```go
// 创建 Informer
informer := cache.NewSharedIndexInformer(
    &cache.ListWatch{
        ListFunc:  func(options metav1.ListOptions) (runtime.Object, error) { ... },
        WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) { ... },
    },
    &v1.Pod{},           // 资源类型
    time.Hour,           // ResyncPeriod
    cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
)

// 注册事件处理器
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    func(obj interface{}) { /* 处理新增 */ },
    UpdateFunc: func(oldObj, newObj interface{}) { /* 处理更新 */ },
    DeleteFunc: func(obj interface{}) { /* 处理删除 */ },
})

// 启动
informer.Run(stopCh)
```

### SharedInformerFactory

为了避免多个 Controller 对同一资源创建多个 Informer（浪费连接），使用 Factory 共享：

```go
factory := informers.NewSharedInformerFactory(clientset, time.Minute)

// 多个 Controller 共享同一个 Pod Informer
podInformer := factory.Core().V1().Pods().Informer()
podLister := factory.Core().V1().Pods().Lister()  // 从 Indexer 查询

factory.Start(stopCh)
factory.WaitForCacheSync(stopCh)
```

## 5. Workqueue（工作队列）

### 作用
Workqueue 是 **限速去重队列**，接收 Informer 的事件，供 Worker 消费。

### 为什么需要 Workqueue？

直接在 EventHandler 中处理有问题：
1. EventHandler 是同步调用，阻塞会影响 Informer
2. 无法重试失败的处理
3. 无法限速（短时间大量事件会打垮下游）
4. 无法去重（同一对象的多次更新需要合并）

### 队列类型

```go
// 1. 普通队列（FIFO + 去重）
queue := workqueue.New()

// 2. 限速队列（推荐）
queue := workqueue.NewRateLimitingQueue(
    workqueue.DefaultControllerRateLimiter(),
)

// 3. 延迟队列
queue := workqueue.NewDelayingQueue()
```

### 限速策略

```go
// 默认限速器：指数退避 + 令牌桶
DefaultControllerRateLimiter() = 
    MaxOfRateLimiter(
        // 指数退避：5ms, 10ms, 20ms, ... 最大 1000s
        BucketRateLimiter(rate.NewLimiter(10, 100)),
        // 令牌桶：10 qps，突发 100
        ItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
    )
```

### 核心操作

```go
// 生产者（EventHandler 调用）
queue.Add(key)                  // 添加到队列
queue.AddAfter(key, duration)   // 延迟添加
queue.AddRateLimited(key)       // 按限速策略添加（用于重试）

// 消费者（Worker 调用）
key, shutdown := queue.Get()    // 阻塞获取
queue.Done(key)                 // 标记处理完成
queue.Forget(key)               // 清除重试计数（成功时调用）
queue.NumRequeues(key)          // 获取重试次数
```

### 去重机制

```
queue.Add("default/pod-a")
queue.Add("default/pod-a")  // 去重，不会重复添加
queue.Add("default/pod-b")

队列内容: ["default/pod-a", "default/pod-b"]

注意：正在处理的 key（Get 后未 Done）再次 Add 会在 Done 后重新入队
```

## 6. Worker（工作协程）

### 作用
Worker 是 **实际的业务处理者**，从 Workqueue 获取 key，执行 Reconcile 逻辑。

### 典型实现

```go
func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
    // 等待缓存同步
    if !cache.WaitForCacheSync(stopCh, c.podsSynced) {
        return fmt.Errorf("failed to sync caches")
    }
    
    // 启动多个 worker
    for i := 0; i < workers; i++ {
        go wait.Until(c.runWorker, time.Second, stopCh)
    }
    
    <-stopCh
    return nil
}

func (c *Controller) runWorker() {
    for c.processNextItem() {
    }
}

func (c *Controller) processNextItem() bool {
    // 1. 从队列获取 key
    key, shutdown := c.queue.Get()
    if shutdown {
        return false
    }
    defer c.queue.Done(key)
    
    // 2. 处理（Reconcile）
    err := c.reconcile(key.(string))
    
    // 3. 处理结果
    if err == nil {
        c.queue.Forget(key)  // 成功，清除重试计数
        return true
    }
    
    // 4. 失败重试
    if c.queue.NumRequeues(key) < maxRetries {
        c.queue.AddRateLimited(key)  // 按限速策略重新入队
        return true
    }
    
    // 5. 超过重试次数，放弃
    c.queue.Forget(key)
    runtime.HandleError(err)
    return true
}
```

### Reconcile 函数

```go
func (c *Controller) reconcile(key string) error {
    namespace, name, err := cache.SplitMetaNamespaceKey(key)
    if err != nil {
        return err
    }
    
    // 从 Indexer（本地缓存）获取对象，不访问 API Server
    pod, err := c.podLister.Pods(namespace).Get(name)
    if errors.IsNotFound(err) {
        // 对象已删除
        return nil
    }
    if err != nil {
        return err
    }
    
    // 执行调谐逻辑...
    return nil
}
```

## 7. 完整数据流

```
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│   ① API Server                                                          │
│      │                                                                   │
│      │ List/Watch (HTTP 长连接)                                          │
│      ▼                                                                   │
│   ② Reflector ──────────────────────────────┐                           │
│      │                                       │                           │
│      │ Add/Update/Delete                     │                           │
│      ▼                                       │                           │
│   ③ DeltaFIFO (Delta 缓冲队列)               │                           │
│      │                                       │                           │
│      │ Pop                                   │                           │
│      ▼                                       │                           │
│   ④ Informer Controller                      │                           │
│      │                                       │                           │
│      ├──────────────────────┐               │                           │
│      │                      │               │                           │
│      │ 更新缓存             │ 触发回调       │                           │
│      ▼                      ▼               │                           │
│   ⑤ Indexer            EventHandler         │                           │
│   (本地缓存)               │                │                           │
│      ▲                     │ Add(key)       │                           │
│      │                     ▼                │                           │
│      │                  ⑥ Workqueue         │                           │
│      │                  (限速队列)           │                           │
│      │                     │                │                           │
│      │                     │ Get            │                           │
│      │                     ▼                │                           │
│      │                  ⑦ Worker            │                           │
│      │                     │                │                           │
│      └─────────────────────┘                │                           │
│           Lister.Get()    Reconcile         │                           │
│         (读缓存，不访问 API)                  │ ResyncPeriod               │
│                                             │ (定期全量同步)              │
│                                             │                           │
│                            ┌────────────────┘                           │
│                            │                                            │
│                            ▼                                            │
│                     定时触发 Update 事件                                  │
│                     确保最终一致性                                        │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

## 8. 关键设计思想

### Level-Triggered vs Edge-Triggered

Kubernetes 采用 **Level-Triggered**（水平触发）设计：

```
Edge-Triggered（边缘触发）：
- 只在状态变化时触发
- 问题：如果处理失败，没有机会重试

Level-Triggered（水平触发）：
- 只要"当前状态"与"期望状态"不一致就触发
- 优点：天然支持重试和最终一致性
```

### 为什么 Reconcile 只接收 Key？

```go
// 不是这样
func Reconcile(obj *v1.Pod) error

// 而是这样
func Reconcile(key string) error
```

原因：
1. **去重**：多个事件合并为一次处理
2. **最新状态**：从 Indexer 获取的永远是最新状态
3. **幂等性**：处理逻辑基于当前状态，不依赖事件类型

### ResyncPeriod 的作用

```go
informer := cache.NewSharedIndexInformer(..., 30*time.Minute, ...)
```

定期将 Indexer 中的所有对象重新入队，用于：
1. 修复可能遗漏的事件
2. 处理外部系统变化（如依赖的 ConfigMap 更新）
3. 保证最终一致性

## 9. Controller-Runtime 封装

本项目使用 controller-runtime，它在 client-go 之上提供了更高级的抽象：

```go
// client-go 原生方式
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    func(obj interface{}) { queue.Add(key) },
    UpdateFunc: func(old, new interface{}) { queue.Add(key) },
    DeleteFunc: func(obj interface{}) { queue.Add(key) },
})

// controller-runtime 方式
ctrl.NewControllerManagedBy(mgr).
    For(&corev1.ConfigMap{}).      // 主资源
    Owns(&corev1.Secret{}).        // 从属资源
    Complete(reconciler)
```

controller-runtime 自动处理：
- Informer 创建和共享
- Workqueue 管理
- Worker 启动和生命周期
- 事件过滤和映射

## 10. 常见问题

### Q: 为什么从 Lister 而不是 API Server 读取？

```go
// ❌ 直接访问 API Server（慢，且可能触发限流）
pod, err := clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})

// ✅ 从本地缓存读取
pod, err := podLister.Pods(ns).Get(name)
```

Informer 保证缓存的最终一致性，读缓存性能高且不会给 API Server 造成压力。

### Q: 对象删除后，Reconcile 中 Get 返回什么？

```go
obj, err := lister.Get(name)
if errors.IsNotFound(err) {
    // 对象已删除，执行清理逻辑
    return nil
}
```

### Q: 如何处理跨资源依赖？

使用 `Watches` 监听关联资源，并映射到主资源的 key：

```go
ctrl.NewControllerManagedBy(mgr).
    For(&MyApp{}).
    Watches(
        &corev1.Secret{},
        handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
            // 查找引用此 Secret 的 MyApp
            return findMyAppsUsingSecret(obj)
        }),
    ).
    Complete(r)
```

## 11. Controller 的核心闭环

Controller 的核心是“读期望状态 + 对比当前状态 + 调谐到一致”。这个闭环决定了：

1. **Reconcile 必须幂等**：同一个 key 可能被重复入队，逻辑要能安全重复执行。
2. **以状态为中心**：不要依赖事件类型，而是从缓存读取最新对象。
3. **最终一致性**：允许中间态和短暂失败，通过重试和 resync 收敛。

典型闭环：

```
observe desired state (spec)
observe actual state (cache/API)
if drift: apply changes
record status
requeue on errors or resync
```

## 12. 队列、并发与限流

### 并发模型

- 一个 controller 可以启动多个 worker 并发处理不同 key。
- 同一 key 在队列中去重，但**可能被并发处理**（例如处理过程中又被 Add）。
- 解决方式：保证 Reconcile 幂等，避免依赖短期假设。

### 限流与重试

- `AddRateLimited` 提供指数退避。
- 失败后重试，成功后 `Forget` 清理计数。
- 建议设置最大重试次数，避免永远阻塞。

## 13. Cache 同步与一致性

- controller 启动时必须 `WaitForCacheSync`。
- 未同步前直接处理会读到空缓存，导致误判删除。
- 对于强一致读（例如立即读取刚写入对象），需要从 API Server 读取。

## 14. 常见最佳实践

1. **区分 Spec 与 Status**：调谐 spec，写回 status 表达当前进度。
2. **OwnerReference + Finalizer**：保证子资源清理与级联删除。
3. **事件过滤**：用 Predicate 降低无效调谐。
4. **小步快跑**：每次 Reconcile 做最少必要变更。
5. **可观测性**：日志里包含 key、错误原因和重试次数。

## 15. 与本文项目的对应关系

在本项目中：

- **Reflector/Informer/Indexer** 由 controller-runtime 自动创建和共享。
- **Workqueue/Worker** 由 controller-runtime 管理并驱动你的 `Reconcile`。
- `SetupWithManager` 决定了 watch 的资源类型与事件来源。
- `Reconcile` 是业务逻辑入口，必须幂等，依赖缓存读取。

如果你需要，我可以补一节“controller-runtime 内部对象对应表”，把 client-go 的类型与 controller-runtime 组件一一对齐。
