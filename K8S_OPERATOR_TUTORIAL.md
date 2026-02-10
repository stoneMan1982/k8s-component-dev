# Kubernetes Operator/Controller 开发教程 (Go)

## 目录
1. [核心概念](#1-核心概念)
2. [开发环境搭建](#2-开发环境搭建)
3. [Kubebuilder 快速入门](#3-kubebuilder-快速入门)
4. [实战：创建第一个 Operator](#4-实战创建第一个-operator)
5. [深入理解 Controller 模式](#5-深入理解-controller-模式)
6. [测试与调试](#6-测试与调试)
7. [部署到集群](#7-部署到集群)
8. [进阶主题](#8-进阶主题)
9. [学习资源](#9-学习资源)

---

## 1. 核心概念

### 1.1 什么是 Kubernetes Operator？

Operator 是一种扩展 Kubernetes API 的软件，用于创建、配置和管理复杂应用的实例。它将运维知识编码到软件中，实现自动化管理。

**核心组件：**
- **CRD (Custom Resource Definition)**：定义自定义资源的 schema
- **CR (Custom Resource)**：CRD 的实例，存储在 etcd 中
- **Controller**：监听资源变化并执行 reconcile 逻辑

### 1.2 Reconciliation Loop（调谐循环）

```
┌─────────────────────────────────────────────────────────┐
│                   Reconciliation Loop                   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│   ┌──────────┐    ┌──────────┐    ┌──────────────────┐  │
│   │  Watch   │───▶│  Event   │───▶│    Reconcile()  │  │
│   │ Resource │    │  Queue   │    │ (核心业务逻辑)    │  │
│   └──────────┘    └──────────┘    └──────────────────┘  │
│        │                                    │           │
│        │         期望状态 vs 实际状态         │          │
│        │                                    ▼           │
│        │                          ┌──────────────────┐  │
│        └──────────────────────────│  Update Status   │  │
│                                   └──────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**核心原则：** 声明式 + 最终一致性
- 用户声明"期望状态"
- Controller 不断将"实际状态"调谐到"期望状态"

### 1.3 Kubebuilder vs Operator SDK

| 特性 | Kubebuilder | Operator SDK |
|------|-------------|--------------|
| 维护者 | Kubernetes SIG | Red Hat |
| 语言支持 | Go | Go, Ansible, Helm |
| 复杂度 | 较简单 | 功能更多 |
| 推荐场景 | 初学者、纯 Go 项目 | 需要多语言支持 |

**推荐：** 初学者使用 Kubebuilder，它更简洁且是 Operator SDK 的基础。

---

## 2. 开发环境搭建

### 2.1 前置要求

```powershell
# 检查 Go 版本 (需要 1.20+)
go version

# 检查 Docker
docker version

# 检查 kubectl
kubectl version --client
```

### 2.2 安装 Kubebuilder (Windows 需要 WSL2)

> ⚠️ **重要：** Kubebuilder 官方不支持 Windows，需要使用 WSL2。

**WSL2 中安装：**
```bash
# 下载 kubebuilder
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder
sudo mv kubebuilder /usr/local/bin/

# 验证安装
kubebuilder version
```

### 2.3 本地 K8s 集群

推荐使用以下方式之一：

**方式一：Kind (推荐)**
```bash
# 安装 Kind
go install sigs.k8s.io/kind@latest

# 创建集群
kind create cluster --name dev

# 验证
kubectl cluster-info
```

**方式二：Docker Desktop Kubernetes**
- 在 Docker Desktop 设置中启用 Kubernetes

**方式三：Minikube**
```bash
minikube start
```

---

## 3. Kubebuilder 快速入门

### 3.1 初始化项目

```bash
# 创建项目目录
mkdir -p ~/projects/my-operator && cd ~/projects/my-operator

# 初始化项目
kubebuilder init --domain example.com --repo github.com/yourname/my-operator

# 查看生成的结构
tree .
```

**生成的目录结构：**
```
my-operator/
├── Dockerfile           # 容器化构建
├── Makefile             # 构建、测试、部署命令
├── PROJECT              # 项目元数据
├── cmd/
│   └── main.go          # 入口文件
├── config/              # K8s 部署配置
│   ├── default/
│   ├── manager/
│   ├── rbac/
│   └── ...
├── go.mod
├── go.sum
└── internal/
    └── controller/      # Controller 实现
```

### 3.2 创建 API (CRD + Controller)

```bash
# 创建 API
kubebuilder create api --group apps --version v1alpha1 --kind MyApp

# 选择：
# Create Resource [y/n]: y  (创建 CRD)
# Create Controller [y/n]: y  (创建 Controller)
```

**新增的文件：**
```
api/
└── v1alpha1/
    ├── groupversion_info.go  # API Group/Version 信息
    ├── myapp_types.go        # ⭐ CRD 类型定义
    └── zz_generated.deepcopy.go

internal/controller/
└── myapp_controller.go       # ⭐ Controller 实现
```

---

## 4. 实战：创建第一个 Operator

### 4.1 场景：Website Operator

创建一个自动部署静态网站的 Operator：
- 用户定义 `Website` CR，指定域名和镜像
- Operator 自动创建 Deployment 和 Service

### 4.2 定义 CRD 类型

编辑 `api/v1alpha1/website_types.go`：

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebsiteSpec 定义 Website 的期望状态
type WebsiteSpec struct {
    // Image 是网站容器镜像
    // +kubebuilder:validation:Required
    Image string `json:"image"`

    // Replicas 是副本数
    // +kubebuilder:default=1
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=10
    Replicas int32 `json:"replicas,omitempty"`

    // Port 是服务端口
    // +kubebuilder:default=80
    Port int32 `json:"port,omitempty"`
}

// WebsiteStatus 定义 Website 的实际状态
type WebsiteStatus struct {
    // AvailableReplicas 是可用副本数
    AvailableReplicas int32 `json:"availableReplicas,omitempty"`

    // Conditions 是状态条件
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableReplicas"

// Website 是 websites API 的 Schema
type Website struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   WebsiteSpec   `json:"spec,omitempty"`
    Status WebsiteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebsiteList 包含 Website 列表
type WebsiteList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Website `json:"items"`
}

func init() {
    SchemeBuilder.Register(&Website{}, &WebsiteList{})
}
```

### 4.3 实现 Controller

编辑 `internal/controller/website_controller.go`：

```go
package controller

import (
    "context"
    "fmt"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    myappsv1alpha1 "github.com/yourname/my-operator/api/v1alpha1"
)

// WebsiteReconciler reconciles a Website object
type WebsiteReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.example.com,resources=websites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.example.com,resources=websites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.example.com,resources=websites/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *WebsiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // 1. 获取 Website CR
    website := &myappsv1alpha1.Website{}
    if err := r.Get(ctx, req.NamespacedName, website); err != nil {
        if errors.IsNotFound(err) {
            logger.Info("Website resource not found, ignoring")
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // 2. 创建或更新 Deployment
    deployment := r.deploymentForWebsite(website)
    if err := ctrl.SetControllerReference(website, deployment, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }

    foundDeployment := &appsv1.Deployment{}
    err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
    if err != nil && errors.IsNotFound(err) {
        logger.Info("Creating Deployment", "name", deployment.Name)
        if err := r.Create(ctx, deployment); err != nil {
            return ctrl.Result{}, err
        }
    } else if err == nil {
        // 更新 Deployment
        if foundDeployment.Spec.Replicas != &website.Spec.Replicas {
            foundDeployment.Spec.Replicas = &website.Spec.Replicas
            if err := r.Update(ctx, foundDeployment); err != nil {
                return ctrl.Result{}, err
            }
        }
    }

    // 3. 创建 Service
    service := r.serviceForWebsite(website)
    if err := ctrl.SetControllerReference(website, service, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }

    foundService := &corev1.Service{}
    err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
    if err != nil && errors.IsNotFound(err) {
        logger.Info("Creating Service", "name", service.Name)
        if err := r.Create(ctx, service); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 4. 更新 Status
    website.Status.AvailableReplicas = foundDeployment.Status.AvailableReplicas
    if err := r.Status().Update(ctx, website); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

// deploymentForWebsite 返回 Website 对应的 Deployment
func (r *WebsiteReconciler) deploymentForWebsite(w *myappsv1alpha1.Website) *appsv1.Deployment {
    labels := map[string]string{"app": w.Name}
    replicas := w.Spec.Replicas

    return &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      w.Name,
            Namespace: w.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &replicas,
            Selector: &metav1.LabelSelector{
                MatchLabels: labels,
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: labels,
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{{
                        Name:  "website",
                        Image: w.Spec.Image,
                        Ports: []corev1.ContainerPort{{
                            ContainerPort: w.Spec.Port,
                        }},
                    }},
                },
            },
        },
    }
}

// serviceForWebsite 返回 Website 对应的 Service
func (r *WebsiteReconciler) serviceForWebsite(w *myappsv1alpha1.Website) *corev1.Service {
    return &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-svc", w.Name),
            Namespace: w.Namespace,
        },
        Spec: corev1.ServiceSpec{
            Selector: map[string]string{"app": w.Name},
            Ports: []corev1.ServicePort{{
                Port: w.Spec.Port,
            }},
        },
    }
}

// SetupWithManager sets up the controller with the Manager
func (r *WebsiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&myappsv1alpha1.Website{}).
        Owns(&appsv1.Deployment{}).
        Owns(&corev1.Service{}).
        Complete(r)
}
```

### 4.4 生成代码和 Manifests

```bash
# 生成 DeepCopy 方法
make generate

# 生成 CRD manifests
make manifests
```

### 4.5 测试 CR

创建 `config/samples/apps_v1alpha1_website.yaml`：

```yaml
apiVersion: apps.example.com/v1alpha1
kind: Website
metadata:
  name: my-website
  namespace: default
spec:
  image: nginx:latest
  replicas: 2
  port: 80
```

---

## 5. 深入理解 Controller 模式

### 5.1 Kubebuilder Markers（注解）

```go
// CRD 相关
// +kubebuilder:object:root=true              // 标记为 root object
// +kubebuilder:subresource:status            // 启用 status 子资源
// +kubebuilder:resource:scope=Cluster        // Cluster-scoped 资源
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// 字段验证
// +kubebuilder:validation:Required           // 必填字段
// +kubebuilder:validation:Minimum=1          // 最小值
// +kubebuilder:validation:Maximum=100        // 最大值
// +kubebuilder:validation:Enum=A;B;C         // 枚举值
// +kubebuilder:default=10                    // 默认值

// RBAC
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
```

### 5.2 Owner References（所有者引用）

```go
// 设置 Owner Reference，实现级联删除
if err := ctrl.SetControllerReference(website, deployment, r.Scheme); err != nil {
    return ctrl.Result{}, err
}
```

当 Website CR 被删除时，其拥有的 Deployment 和 Service 会自动删除。

### 5.3 Finalizers（终结器）

用于在资源删除前执行清理逻辑：

```go
const websiteFinalizer = "apps.example.com/finalizer"

func (r *WebsiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    website := &myappsv1alpha1.Website{}
    if err := r.Get(ctx, req.NamespacedName, website); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 检查是否在删除中
    if !website.DeletionTimestamp.IsZero() {
        if containsString(website.GetFinalizers(), websiteFinalizer) {
            // 执行清理逻辑
            if err := r.cleanupExternalResources(website); err != nil {
                return ctrl.Result{}, err
            }
            // 移除 finalizer
            website.SetFinalizers(removeString(website.GetFinalizers(), websiteFinalizer))
            if err := r.Update(ctx, website); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // 添加 finalizer
    if !containsString(website.GetFinalizers(), websiteFinalizer) {
        website.SetFinalizers(append(website.GetFinalizers(), websiteFinalizer))
        if err := r.Update(ctx, website); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ... 正常 reconcile 逻辑
    return ctrl.Result{}, nil
}
```

### 5.4 Requeue（重新入队）

```go
// 立即重试
return ctrl.Result{Requeue: true}, nil

// 延迟重试
return ctrl.Result{RequeueAfter: time.Second * 30}, nil

// 成功，不重试
return ctrl.Result{}, nil

// 错误，自动重试（指数退避）
return ctrl.Result{}, err
```

---

## 6. 测试与调试

### 6.1 本地运行

```bash
# 安装 CRD 到集群
make install

# 本地运行 Controller（不部署到集群）
make run
```

### 6.2 单元测试

Kubebuilder 使用 Ginkgo + envtest：

```go
// internal/controller/suite_test.go
var _ = BeforeSuite(func() {
    // 启动 envtest（模拟 API Server + etcd）
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
    }
    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    // ...
})

// internal/controller/website_controller_test.go
var _ = Describe("Website Controller", func() {
    Context("When creating a Website", func() {
        It("Should create a Deployment", func() {
            // 测试逻辑
        })
    })
})
```

运行测试：
```bash
make test
```

### 6.3 调试技巧

```go
// 使用 logger
logger := log.FromContext(ctx)
logger.Info("Processing website", "name", website.Name, "replicas", website.Spec.Replicas)
logger.Error(err, "Failed to create deployment")

// 使用 Events
r.Recorder.Event(website, corev1.EventTypeNormal, "Created", "Deployment created successfully")
r.Recorder.Event(website, corev1.EventTypeWarning, "Failed", fmt.Sprintf("Failed: %v", err))
```

---

## 7. 部署到集群

### 7.1 构建镜像

```bash
# 构建并推送镜像
make docker-build docker-push IMG=your-registry/my-operator:v0.1.0
```

### 7.2 部署到集群

```bash
# 部署 CRD 和 Controller
make deploy IMG=your-registry/my-operator:v0.1.0

# 查看部署状态
kubectl get pods -n my-operator-system
```

### 7.3 卸载

```bash
# 卸载 Controller
make undeploy

# 卸载 CRD
make uninstall
```

---

## 8. 进阶主题

### 8.1 Webhook（准入控制）

```bash
# 创建 Webhook
kubebuilder create webhook --group apps --version v1alpha1 --kind Website --defaulting --programmatic-validation
```

**Validating Webhook：** 验证资源是否合法
**Mutating Webhook：** 修改资源（如设置默认值）

### 8.2 多版本 API

```bash
# 创建新版本
kubebuilder create api --group apps --version v1beta1 --kind Website
```

需要实现版本转换（Conversion Webhook）。

### 8.3 Leader Election

生产环境部署多副本时需要：

```go
// cmd/main.go
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    LeaderElection:   true,
    LeaderElectionID: "my-operator.example.com",
})
```

### 8.4 Metrics 和监控

Kubebuilder 默认暴露 Prometheus metrics：
- `/metrics` 端点
- 自定义 metrics 可通过 `prometheus/client_golang` 添加

---

## 9. 学习资源

### 官方文档
- [Kubebuilder Book](https://book.kubebuilder.io/) - 官方教程
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime) - API 文档
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)

### 优秀开源 Operator 参考
- [cert-manager](https://github.com/cert-manager/cert-manager) - 证书管理
- [prometheus-operator](https://github.com/prometheus-operator/prometheus-operator) - 监控
- [external-secrets](https://github.com/external-secrets/external-secrets) - 密钥管理

### 社区
- Kubernetes Slack: `#kubebuilder`
- [CNCF Operator Framework](https://operatorframework.io/)

---

## 快速参考：常用命令

```bash
# 初始化项目
kubebuilder init --domain example.com --repo github.com/yourname/my-operator

# 创建 API
kubebuilder create api --group apps --version v1alpha1 --kind MyResource

# 创建 Webhook
kubebuilder create webhook --group apps --version v1alpha1 --kind MyResource --defaulting --programmatic-validation

# 生成代码
make generate    # 生成 DeepCopy
make manifests   # 生成 CRD/RBAC/Webhook manifests

# 开发
make install     # 安装 CRD
make run         # 本地运行
make test        # 运行测试

# 部署
make docker-build docker-push IMG=xxx
make deploy IMG=xxx

# 清理
make undeploy
make uninstall
```
