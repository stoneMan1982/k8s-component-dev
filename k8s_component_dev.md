# Kubernetes 自定义组件开发全览

---

## 1. Custom Resource(CRD) + Controller

### 1.1 概述
- **目的**：扩展 Kubernetes API，自动化管理自定义资源
- **工具**：Kubebuilder、Operator SDK

### 1.2 核心概念
```
┌─────────────────────────────────────────────────────┐
│                     Operator 模式                    │
├─────────────────────────────────────────────────────┤
│  CRD (定义)          CR (实例)          Controller   │
│  ┌───────────┐     ┌───────────┐     ┌───────────┐  │
│  │ Schema    │────▶│ 期望状态  │────▶│ Reconcile │  │
│  │ 字段定义   │     │ spec      │     │ 调谐逻辑   │  │
│  └───────────┘     └───────────┘     └───────────┘  │
└─────────────────────────────────────────────────────┘
```

| 组件 | 说明 |
|-----|-----|
| CRD | 自定义资源定义，类似“表结构” |
| CR | 自定义资源实例，类似“表数据” |
| Controller | 监听 CR 变化，执行调谐逻辑 |
| Reconcile | 将实际状态调谐到期望状态 |

### 1.3 开发步骤
```bash
# 1. 初始化项目
kubebuilder init --domain example.com --repo github.com/yourname/my-operator

# 2. 创建 API
kubebuilder create api --group apps --version v1alpha1 --kind MyApp

# 3. 定义 CRD 字段 (api/v1alpha1/myapp_types.go)
type MyAppSpec struct {
    Replicas int32  `json:"replicas"`
    Image    string `json:"image"`
}

# 4. 实现 Controller (internal/controller/myapp_controller.go)
func (r *MyAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 获取 CR -> 创建/更新资源 -> 更新 Status
}

# 5. 生成代码
make generate manifests

# 6. 运行测试
make install  # 安装 CRD
make run      # 本地运行
```

### 1.4 关键代码示例
```go
// Reconcile 核心逻辑
func (r *MyAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取 CR
    myApp := &appsv1alpha1.MyApp{}
    if err := r.Get(ctx, req.NamespacedName, myApp); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 创建所需资源 (Deployment/Service 等)
    deployment := r.constructDeployment(myApp)
    if err := controllerutil.SetControllerReference(myApp, deployment, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }
    
    // 3. CreateOrUpdate 模式
    _, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
        deployment.Spec.Replicas = &myApp.Spec.Replicas
        return nil
    })

    // 4. 更新 Status
    myApp.Status.AvailableReplicas = deployment.Status.AvailableReplicas
    r.Status().Update(ctx, myApp)

    return ctrl.Result{}, nil
}
```

### 1.5 应用场景
- 数据库自动化运维 (MySQL/Redis/PostgreSQL Operator)
- 中间件管理 (Kafka/RabbitMQ/Elasticsearch Operator)
- 应用部署流水线 (ArgoCD, Flux)
- 证书管理 (cert-manager)
- 密钥管理 (external-secrets)

### 1.6 参考项目
- [sample-controller](https://github.com/kubernetes/sample-controller) - 官方示例
- [kubebuilder](https://book.kubebuilder.io/) - 官方教程
- [cert-manager](https://github.com/cert-manager/cert-manager) - 生产级示例

---

## 2. Custom Scheduler

### 2.1 概述
- **目的**：替代或增强默认调度器
- **工具**：Go + client-go 或 Scheduling Framework

### 2.2 调度流程
```
┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
│  Pod       │────▶│  Filter   │────▶│  Score    │────▶│  Bind     │
│  Pending   │     │  过滤节点   │     │  节点打分   │     │  绑定节点   │
└───────────┘     └───────────┘     └───────────┘     └───────────┘
```

### 2.3 开发方式

#### 方式一：独立调度器 (简单)
```go
func main() {
    clientset := getKubernetesClient()
    
    // 监听未调度的 Pod
    watch, _ := clientset.CoreV1().Pods("").Watch(context.TODO(), metav1.ListOptions{
        FieldSelector: "spec.schedulerName=my-scheduler,spec.nodeName=",
    })
    
    for event := range watch.ResultChan() {
        pod := event.Object.(*corev1.Pod)
        node := selectBestNode(pod)  // 自定义调度逻辑
        bindPodToNode(pod, node)
    }
}

func selectBestNode(pod *corev1.Pod) string {
    nodes := getAvailableNodes()
    // 自定义调度策略
    // 例如：根据 GPU 数量、内存、拓扑等
    return bestNode
}

func bindPodToNode(pod *corev1.Pod, nodeName string) {
    binding := &corev1.Binding{
        ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace},
        Target:     corev1.ObjectReference{Kind: "Node", Name: nodeName},
    }
    clientset.CoreV1().Pods(pod.Namespace).Bind(context.TODO(), binding, metav1.CreateOptions{})
}
```

#### 方式二：Scheduling Framework Plugin (推荐)
```go
// 实现 FilterPlugin 接口
type MyPlugin struct{}

func (p *MyPlugin) Name() string { return "MyPlugin" }

func (p *MyPlugin) Filter(ctx context.Context, state *framework.CycleState, 
    pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
    // 返回节点是否适合
    if !hasRequiredGPU(nodeInfo, pod) {
        return framework.NewStatus(framework.Unschedulable, "insufficient GPU")
    }
    return framework.NewStatus(framework.Success, "")
}

// 实现 ScorePlugin 接口
func (p *MyPlugin) Score(ctx context.Context, state *framework.CycleState,
    pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
    // 返回节点分数 (0-100)
    return calculateScore(nodeName), nil
}
```

### 2.4 使用自定义调度器
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  schedulerName: my-custom-scheduler  # 指定调度器
  containers:
  - name: app
    image: nginx
```

### 2.5 应用场景
- GPU/NPU 特殊硬件调度
- 拓扑感知调度 (同机房/同机架)
- 成本优化调度 (Spot 实例优先)
- 负载均衡调度
- 批量作业调度 (gang scheduling)

### 2.6 参考项目
- [scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins) - 官方插件集
- [volcano](https://github.com/volcano-sh/volcano) - 批量调度器
- [kube-scheduler](https://github.com/kubernetes/kubernetes/tree/master/pkg/scheduler) - 源码

---

## 3. Admission Webhook

### 3.1 概述
- **目的**：拦截 API 请求进行验证或修改
- **工具**：Kubebuilder / 纯 Go

### 3.2 处理流程
```
API 请求 ──▶ Authentication ──▶ Authorization ──▶ Mutating Webhook ──▶ Validating Webhook ──▶ etcd
                                                    (修改请求)           (验证请求)
```

### 3.3 Webhook 类型

| 类型 | 作用 | 执行顺序 |
|-----|-----|--------|
| Mutating | 修改资源（注入 sidecar、设置默认值） | 先执行 |
| Validating | 验证资源（拒绝不合规请求） | 后执行 |

### 3.4 开发示例

#### Kubebuilder 方式
```bash
# 创建 webhook
kubebuilder create webhook --group apps --version v1 --kind Pod --defaulting --programmatic-validation
```

#### 手动实现
```go
// Mutating Webhook - 注入 sidecar
func mutatePod(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
    pod := &corev1.Pod{}
    json.Unmarshal(ar.Request.Object.Raw, pod)

    // 添加 sidecar 容器
    sidecar := corev1.Container{
        Name:  "sidecar",
        Image: "busybox",
    }
    pod.Spec.Containers = append(pod.Spec.Containers, sidecar)

    // 生成 JSON Patch
    patch := []map[string]interface{}{
        {
            "op":    "add",
            "path":  "/spec/containers/-",
            "value": sidecar,
        },
    }
    patchBytes, _ := json.Marshal(patch)

    return &admissionv1.AdmissionResponse{
        Allowed: true,
        Patch:   patchBytes,
        PatchType: func() *admissionv1.PatchType {
            pt := admissionv1.PatchTypeJSONPatch
            return &pt
        }(),
    }
}

// Validating Webhook - 禁止使用 latest 标签
func validatePod(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
    pod := &corev1.Pod{}
    json.Unmarshal(ar.Request.Object.Raw, pod)

    for _, c := range pod.Spec.Containers {
        if strings.HasSuffix(c.Image, ":latest") {
            return &admissionv1.AdmissionResponse{
                Allowed: false,
                Result: &metav1.Status{
                    Message: "Using :latest tag is not allowed",
                },
            }
        }
    }
    return &admissionv1.AdmissionResponse{Allowed: true}
}
```

### 3.5 部署配置
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: my-mutating-webhook
webhooks:
- name: mutate.example.com
  clientConfig:
    service:
      name: webhook-service
      namespace: default
      path: /mutate
    caBundle: ${CA_BUNDLE}
  rules:
  - operations: ["CREATE"]
    apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["pods"]
  admissionReviewVersions: ["v1"]
  sideEffects: None
```

### 3.6 应用场景
- 安全策略执行（禁止 privileged、强制 resource limits）
- Sidecar 自动注入（Istio、Linkerd）
- 默认值设置（自动添加 labels/annotations）
- 镜像仓库重写（内网镜像代理）
- 资源配额验证

### 3.7 参考项目
- [OPA Gatekeeper](https://github.com/open-policy-agent/gatekeeper) - 策略引擎
- [Kyverno](https://github.com/kyverno/kyverno) - K8s 原生策略
- [Istio](https://github.com/istio/istio) - sidecar 注入示例

---

## 4. Custom Metrics Provider

### 4.1 概述
- **目的**：提供自定义指标给 HPA
- **接口**：Custom Metrics API / External Metrics API

### 4.2 指标 API 类型

| API | 用途 | 示例 |
|-----|-----|------|
| metrics.k8s.io | 核心指标 (CPU/内存) | metrics-server |
| custom.metrics.k8s.io | 自定义指标 (关联 K8s 对象) | http_requests_per_second |
| external.metrics.k8s.io | 外部指标 (不关联 K8s 对象) | cloud_queue_length |

### 4.3 架构
```
┌─────────┐     ┌───────────────────┐     ┌─────────────────┐
│   HPA   │────▶│ Custom Metrics  │────▶│    Prometheus   │
│         │     │    Adapter      │     │    / 其他数据源   │
└─────────┘     └───────────────────┘     └─────────────────┘
```

### 4.4 实现接口
```go
import (
    "sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

type MyMetricsProvider struct{}

// 获取某个对象的指标
func (p *MyMetricsProvider) GetMetricByName(ctx context.Context, 
    name types.NamespacedName, info provider.CustomMetricInfo, 
    metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
    
    // 从 Prometheus/其他数据源获取指标
    value := queryPrometheus(name, info.Metric)
    
    return &custom_metrics.MetricValue{
        DescribedObject: custom_metrics.ObjectReference{
            Kind:      info.GroupResource.Resource,
            Name:      name.Name,
            Namespace: name.Namespace,
        },
        Metric: custom_metrics.MetricIdentifier{
            Name: info.Metric,
        },
        Value: *resource.NewQuantity(value, resource.DecimalSI),
    }, nil
}

// 列出所有可用指标
func (p *MyMetricsProvider) ListAllMetrics() []provider.CustomMetricInfo {
    return []provider.CustomMetricInfo{
        {GroupResource: schema.GroupResource{Resource: "pods"}, Metric: "http_requests_per_second", Namespaced: true},
        {GroupResource: schema.GroupResource{Resource: "pods"}, Metric: "queue_length", Namespaced: true},
    }
}
```

### 4.5 HPA 配置示例
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-app-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 1
  maxReplicas: 10
  metrics:
  # 自定义指标
  - type: Pods
    pods:
      metric:
        name: http_requests_per_second
      target:
        type: AverageValue
        averageValue: 100
  # 外部指标
  - type: External
    external:
      metric:
        name: queue_messages
        selector:
          matchLabels:
            queue: my-queue
      target:
        type: AverageValue
        averageValue: 30
```

### 4.6 应用场景
- 基于 QPS 的自动扩容
- 队列消息数量监控
- 业务指标驱动扩缩容
- 外部系统指标集成

### 4.7 参考项目
- [prometheus-adapter](https://github.com/kubernetes-sigs/prometheus-adapter) - Prometheus 指标适配器
- [KEDA](https://github.com/kedacore/keda) - 事件驱动自动扩容
- [custom-metrics-apiserver](https://github.com/kubernetes-sigs/custom-metrics-apiserver) - 官方框架

---

## 5. Device Plugin

### 5.1 概述
- **目的**：管理节点上的特殊硬件设备
- **接口**：Device Plugin API (gRPC)
- **难度**：★★★☆☆

### 5.2 架构
```
┌───────────────────────────────────────────┐
│                    Node                           │
│  ┌─────────────┐     ┌───────────────────┐      │
│  │   kubelet   │◀────│  Device Plugin    │      │
│  └─────────────┘     │  (gRPC Server)    │      │
│        │             └─────────┬─────────┘      │
│        │                       │                │
│        ▼                       ▼                │
│  ┌─────────────┐     ┌───────────────────┐      │
│  │   Pod       │     │  GPU/FPGA/特殊硬件  │      │
│  └─────────────┘     └───────────────────┘      │
└───────────────────────────────────────────┘
```

### 5.3 实现接口
```go
import (
    pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type MyDevicePlugin struct {
    devices []*pluginapi.Device
}

// 注册到 kubelet
func (p *MyDevicePlugin) Register() error {
    conn, _ := grpc.Dial(pluginapi.KubeletSocket, grpc.WithInsecure())
    client := pluginapi.NewRegistrationClient(conn)
    
    _, err := client.Register(context.TODO(), &pluginapi.RegisterRequest{
        Version:      pluginapi.Version,
        Endpoint:     "my-device.sock",
        ResourceName: "example.com/my-device",  // 资源名称
    })
    return err
}

// 列出可用设备
func (p *MyDevicePlugin) ListAndWatch(e *pluginapi.Empty, 
    s pluginapi.DevicePlugin_ListAndWatchServer) error {
    
    for {
        devices := p.discoverDevices()  // 发现设备
        s.Send(&pluginapi.ListAndWatchResponse{Devices: devices})
        time.Sleep(10 * time.Second)
    }
}

// 分配设备给容器
func (p *MyDevicePlugin) Allocate(ctx context.Context, 
    req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
    
    responses := &pluginapi.AllocateResponse{}
    for _, r := range req.ContainerRequests {
        response := &pluginapi.ContainerAllocateResponse{
            // 设备映射
            Devices: []*pluginapi.DeviceSpec{
                {
                    ContainerPath: "/dev/my-device",
                    HostPath:      "/dev/my-device-0",
                    Permissions:   "rw",
                },
            },
            // 环境变量
            Envs: map[string]string{
                "MY_DEVICE_ID": r.DevicesIDs[0],
            },
        }
        responses.ContainerResponses = append(responses.ContainerResponses, response)
    }
    return responses, nil
}
```

### 5.4 Pod 使用设备
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-pod
spec:
  containers:
  - name: app
    image: nvidia/cuda
    resources:
      limits:
        example.com/my-device: 1  # 请求 1 个设备
```

### 5.5 应用场景
- GPU 管理 (NVIDIA, AMD)
- NPU/TPU 管理
- FPGA 管理
- SR-IOV 网卡
- 加密卡/HSM

### 5.6 参考项目
- [NVIDIA device plugin](https://github.com/NVIDIA/k8s-device-plugin)
- [AMD GPU device plugin](https://github.com/RadeonOpenCompute/k8s-device-plugin)
- [Intel device plugins](https://github.com/intel/intel-device-plugins-for-kubernetes)

---

## 6. CNI Plugin

### 6.1 概述
- **目的**：实现容器网络接口
- **接口**：CNI 规范 (JSON 配置 + 可执行文件)

### 6.2 CNI 工作流程
```
kubelet ──▶ CRI (containerd) ──▶ CNI Plugin ──▶ 配置网络
                                      │
                                      ├── ADD: 创建网络接口
                                      ├── DEL: 删除网络接口
                                      └── CHECK: 检查网络状态
```

### 6.3 CNI 插件实现
```go
import (
    "github.com/containernetworking/cni/pkg/skel"
    "github.com/containernetworking/cni/pkg/types"
    current "github.com/containernetworking/cni/pkg/types/100"
)

type NetConf struct {
    types.NetConf
    Bridge string `json:"bridge"`
    Subnet string `json:"subnet"`
}

func cmdAdd(args *skel.CmdArgs) error {
    // 1. 解析配置
    conf := &NetConf{}
    json.Unmarshal(args.StdinData, conf)

    // 2. 创建 veth pair
    hostVeth, containerVeth, _ := ip.SetupVeth(args.IfName, 1500, "", 
        func(hostNS ns.NetNS) error {
            return nil
        })

    // 3. 将 veth 一端放入容器网络命名空间
    containerNS, _ := ns.GetNS(args.Netns)
    containerNS.Do(func(ns.NetNS) error {
        // 配置 IP 地址
        ipConfig := &current.IPConfig{
            Address: getNextIP(conf.Subnet),
            Gateway: getGateway(conf.Subnet),
        }
        return configureInterface(containerVeth, ipConfig)
    })

    // 4. 将另一端连接到网桥
    br, _ := netlink.LinkByName(conf.Bridge)
    netlink.LinkSetMaster(hostVeth, br)

    // 5. 返回结果
    result := &current.Result{
        CNIVersion: conf.CNIVersion,
        IPs:        []*current.IPConfig{ipConfig},
    }
    return types.PrintResult(result, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
    // 清理网络接口
    return nil
}

func main() {
    skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("my-cni"))
}
```

### 6.4 CNI 配置文件
```json
// /etc/cni/net.d/10-mynet.conf
{
  "cniVersion": "1.0.0",
  "name": "mynet",
  "type": "my-cni",
  "bridge": "cni0",
  "subnet": "10.244.0.0/16",
  "ipam": {
    "type": "host-local",
    "ranges": [[{"subnet": "10.244.0.0/24"}]]
  }
}
```

### 6.5 应用场景
- 自定义 Overlay 网络 (VXLAN, Geneve)
- 多网络平面 (Multus)
- 网络策略增强
- 高性能网络 (DPDK, SR-IOV)
- 服务网格集成

### 6.6 参考项目
- [CNI Plugins](https://github.com/containernetworking/plugins) - 官方插件集
- [Calico](https://github.com/projectcalico/calico) - 流行 CNI
- [Cilium](https://github.com/cilium/cilium) - eBPF-based CNI
- [Flannel](https://github.com/flannel-io/flannel) - 简单 Overlay

---

## 7. CSI Plugin

### 7.1 概述
- **目的**：实现容器存储接口
- **接口**：CSI 规范 (gRPC)

### 7.2 架构
```
┌────────────────────────────────────────────────────────────┐
│                         Kubernetes                          │
│  ┌────────────────┐   ┌────────────────┐   ┌─────────────┐  │
│  │ PVC            │   │ StorageClass   │   │ PV          │  │
│  └────────┬───────┘   └────────┬───────┘   └──────┬──────┘  │
└─────────┼──────────────────┼────────────────────┼─────────┘
           │                  │                    │
           ▼                  ▼                    ▼
┌────────────────────────────────────────────────────────────┐
│                       CSI Driver                            │
│  ┌───────────────┐  ┌───────────────┐  ┌────────────────┐ │
│  │ Controller    │  │ Node Plugin   │  │ Identity       │ │
│  │ (CreateVol)   │  │ (Mount)       │  │ (GetInfo)      │ │
│  └───────────────┘  └───────────────┘  └────────────────┘ │
└────────────────────────────────────────────────────────────┘
           │                  │                    │
           ▼                  ▼                    ▼
┌────────────────────────────────────────────────────────────┐
│                   Storage Backend                           │
│            (云硬盘 / NFS / Ceph / 本地存储)                   │
└────────────────────────────────────────────────────────────┘
```

### 7.3 CSI 接口

| 服务 | 方法 | 说明 |
|-----|-----|------|
| Identity | GetPluginInfo | 返回插件信息 |
| Identity | GetPluginCapabilities | 返回插件能力 |
| Controller | CreateVolume | 创建卷 |
| Controller | DeleteVolume | 删除卷 |
| Controller | ControllerPublishVolume | 挂载卷到节点 |
| Node | NodeStageVolume | 卷格式化、挂载到临时目录 |
| Node | NodePublishVolume | 绑定到 Pod 目录 |

### 7.4 实现示例
```go
import (
    "github.com/container-storage-interface/spec/lib/go/csi"
)

type MyCSIDriver struct {
    csi.UnimplementedIdentityServer
    csi.UnimplementedControllerServer
    csi.UnimplementedNodeServer
}

// Identity 服务
func (d *MyCSIDriver) GetPluginInfo(ctx context.Context, 
    req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
    return &csi.GetPluginInfoResponse{
        Name:          "my-csi-driver",
        VendorVersion: "1.0.0",
    }, nil
}

// Controller 服务 - 创建卷
func (d *MyCSIDriver) CreateVolume(ctx context.Context,
    req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
    
    volumeID := uuid.New().String()
    capacityBytes := req.CapacityRange.RequiredBytes
    
    // 调用存储后端 API 创建卷
    if err := createStorageVolume(volumeID, capacityBytes); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    
    return &csi.CreateVolumeResponse{
        Volume: &csi.Volume{
            VolumeId:      volumeID,
            CapacityBytes: capacityBytes,
        },
    }, nil
}

// Node 服务 - 挂载卷到 Pod
func (d *MyCSIDriver) NodePublishVolume(ctx context.Context,
    req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
    
    volumeID := req.VolumeId
    targetPath := req.TargetPath
    
    // 挂载存储卷
    if err := mount.New("").Mount(getDevicePath(volumeID), targetPath, "ext4", nil); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    
    return &csi.NodePublishVolumeResponse{}, nil
}
```

### 7.5 部署组件
```yaml
# CSI Driver 注册
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: my-csi-driver
spec:
  attachRequired: true
  podInfoOnMount: true
---
# StorageClass
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: my-storage
provisioner: my-csi-driver
parameters:
  type: ssd
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
```

### 7.6 应用场景
- 对接云存储服务 (AWS EBS, Azure Disk, GCE PD)
- 本地存储管理 (local-path, TopoLVM)
- 分布式存储系统 (Ceph, GlusterFS, Longhorn)
- 网络存储 (NFS, SMB)

### 7.7 参考项目
- [CSI Spec](https://github.com/container-storage-interface/spec) - 官方规范
- [csi-driver-nfs](https://github.com/kubernetes-csi/csi-driver-nfs) - NFS 示例
- [Longhorn](https://github.com/longhorn/longhorn) - 云原生存储
- [Rook-Ceph](https://github.com/rook/rook) - Ceph 管理

---

## 8. 总结对比

| 组件类型 | 难度 | 开发语言 | 主要接口 | 典型场景 |
|---------|------|---------|---------|----------|
| CRD + Controller | ★★ | Go | controller-runtime | 应用自动化管理 |
| Custom Scheduler | ★★★★ | Go | Scheduling Framework | 特殊调度策略 |
| Admission Webhook | ★★★ | Go | AdmissionReview | 安全策略/注入 |
| Metrics Provider | ★★★ | Go | Custom Metrics API | 自定义 HPA |
| Device Plugin | ★★★ | Go | Device Plugin API | GPU/硬件管理 |
| CNI Plugin | ★★★★★ | Go | CNI Spec | 自定义网络 |
| CSI Plugin | ★★★★ | Go | CSI Spec | 自定义存储 |

---

## 9. 学习路径建议

```
初级 (1-2周)
├── Go 语言基础
├── Kubernetes 核心概念
└── kubectl 基本操作

中级 (2-4周)
├── client-go 使用
├── Kubebuilder 创建 Operator  ← 建议起点
└── Admission Webhook

高级 (1-2月)
├── Custom Scheduler
├── Custom Metrics Provider
└── Device Plugin

专家 (3月+)
├── CNI Plugin
└── CSI Plugin
```

**推荐学习顺序**：CRD+Controller → Webhook → Metrics → Scheduler → Device Plugin → CSI → CNI

