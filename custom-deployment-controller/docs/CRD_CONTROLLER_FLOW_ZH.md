# 开发kubernetes CRD 和 Controller的完整流程  

## 整体流程概览

```text
CRD定义 --> 生成代码 --> 实现业务逻辑 --> 部署运行
```

- 确定需要解决的问题和CRD的用途  
- 设计API结构（Spec 和 Status 字段）
- 考虑版本管理策略  
- 指定Controller的协调逻辑 

## 具体开发流程  

### 定义CRD（API类型）（API Schema）  

```go

// 定义期望的状态
type MyResourceSpec struct {
    Image    string `json:"image"`
    Replicas int32  `json:"replicas"`
}

// 定义运行时Watch的状态  
type MyResourceStatus struct {
    Ready    bool `json:"ready"`
    Message  string `json:"message"`
}

// MyResource 资源定义
type MyResource struct {
    metav1.TypeMeta            `json:",inline"`
    metav1.ObjectMeta          `json:"metadata,omitempty"`

    Spec    MyResourceSpec       `json:"spec,omitempty"`
    Status  MyResourceStatus     `json:"status,omitempty"`
}


// MyResourceList  MyResource 的列表
type MyResourceList struct {
    metav1.TypeMeta            `json:",inline"`
    metav1.ObjectMeta          `json:"metadata,omitempty"`

    Items   []MyResource       `json:"items"`
}

```

### 生成CRD文件

```makefile
.PHONY: manifests
manifests: controller-gen
    $(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./..." output:crd:artifacts:config=config/crd

.PHONY: generate
generate: controller-gen
    $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
```

运行命令生成CRD文件：

```bash
make manifests
make generate
```

### 实现Controller逻辑  

核心代码如下：

```go
func (c *CustomDeploymentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cd := &appsv1alpha1.CustomDeployment{}
	if err := c.Get(ctx, req.NamespacedName, cd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	deployName := cd.Name
	deploy := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: deployName, Namespace: cd.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		// 创建 Deployment
		deploy = desiredDeployment(cd)
		if err := ctrl.SetControllerReference(cd, deploy, c.Scheme); err != nil {
			logger.Error(err, "Failed to set owner reference")
			return ctrl.Result{}, err
		}
		if err := c.Create(ctx, deploy); err != nil {
			logger.Error(err, "Failed to create Deployment")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	} else {
		updated := false
		if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != cd.Spec.Replicas {
			deploy.Spec.Replicas = ptr.To(cd.Spec.Replicas)
			updated = true
		}
		if updated {
			if err := c.Update(ctx, deploy); err != nil {
				logger.Error(err, "Failed to update Deployment")
				return ctrl.Result{}, err
			}

			logger.Info("Deployment updated successfully", "name", deploy.Name)
		}
	}

	if cd.Status.AvailableReplicas != deploy.Status.AvailableReplicas {
		cd.Status.AvailableReplicas = deploy.Status.AvailableReplicas
		if err := c.Status().Update(ctx, cd); err != nil {
			logger.Error(err, "Failed to update CustomDeployment status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
```

### 创建 Manager 入口  

```go

func main() {
	// 这里是 main 函数的入口，通常会在这里设置 Manager 和 Controller

	logger := ctrl.Log.WithName("setup")
	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add appsv1alpha1 to scheme")
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add apps/v1 to scheme")
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add core/v1 to scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	reconciler := &controller.CustomDeploymentController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.CustomDeployment{}).
		Owns(&appsv1.Deployment{}).
		Complete(reconciler); err != nil {
		logger.Error(err, "Unable to create controller")
		os.Exit(1)
	}

	logger.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

```

## 总结  

| 概念                | 说明                                              |
| ------------------- | ------------------------------------------------ |
| **CRD**             | API资源定义，告诉k8s如何存储和验证你的自定义对象     |
| **Controller**      | 监听CRD实例，执行期望状态和实际状态的协调            |
| **Reconcile**       | Controller的核心方法，无限                         |
| **Finalizer**       | 用于优雅删除，确保清理相关资源                      |
| **Owner Reference** | 建立资源之间关系，实现级联删除                      |