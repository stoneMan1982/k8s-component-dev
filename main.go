package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// 注解：当 ConfigMap 有这个 annotation 时，会自动同步到 Secret
const syncAnnotation = "simple-controller/sync-to-secret"

// ConfigMapReconciler 监听 ConfigMap 变化
type ConfigMapReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile 是核心调谐逻辑
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// ========== 调试技巧 ==========
	// 1. 基本日志
	logger.Info("Reconcile triggered", "namespace", req.Namespace, "name", req.Name)

	// 2. 带级别的日志 (V(1) = debug, 需要 -zap-log-level=debug 才显示)
	logger.V(1).Info("Debug info", "request", req)

	// 3. 错误日志
	// logger.Error(err, "Something went wrong", "key", "value")
	// ==============================

	// 1. 获取 ConfigMap
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, configMap); err != nil {
		if errors.IsNotFound(err) {
			// ConfigMap 被删除，尝试删除对应的 Secret
			logger.Info("ConfigMap deleted, cleaning up Secret", "name", req.Name)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      req.Name + "-synced",
					Namespace: req.Namespace,
				},
			}
			if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. 检查是否有同步 annotation
	if _, exists := configMap.Annotations[syncAnnotation]; !exists {
		logger.V(1).Info("ConfigMap does not have sync annotation, skipping", "name", configMap.Name)
		return ctrl.Result{}, nil
	}

	logger.Info("Syncing ConfigMap to Secret", "configmap", configMap.Name)

	// 3. 构建对应的 Secret
	secretName := configMap.Name + "-synced"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: configMap.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "simple-controller",
				"app.kubernetes.io/source":     configMap.Name,
			},
		},
		StringData: configMap.Data, // 将 ConfigMap 数据复制到 Secret
	}

	// 设置 OwnerReference，实现级联删除
	if err := ctrl.SetControllerReference(configMap, secret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// 4. 创建或更新 Secret
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: configMap.Namespace}, existingSecret)

	if errors.IsNotFound(err) {
		// Secret 不存在，创建
		logger.Info("Creating Secret", "name", secretName)
		if err := r.Create(ctx, secret); err != nil {
			logger.Error(err, "Failed to create Secret")
			return ctrl.Result{}, err
		}
		logger.Info("✅ Secret created successfully", "name", secretName)
	} else if err == nil {
		// Secret 存在，更新
		existingSecret.StringData = configMap.Data
		existingSecret.Labels = secret.Labels
		logger.Info("Updating Secret", "name", secretName)
		if err := r.Update(ctx, existingSecret); err != nil {
			logger.Error(err, "Failed to update Secret")
			return ctrl.Result{}, err
		}
		logger.Info("✅ Secret updated successfully", "name", secretName)
	} else {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager 注册 Controller
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).      // 监听 ConfigMap
		Owns(&corev1.Secret{}).        // 也监听它创建的 Secret
		Complete(r)
}

func main() {
	var metricsAddr string
	var namespace string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&namespace, "namespace", "", "Namespace to watch (empty = all namespaces)")
	flag.Parse()

	// 设置日志
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := ctrl.Log.WithName("setup")

	// 创建 Manager
	options := ctrl.Options{
		Scheme:         runtime.NewScheme(),
		LeaderElection: false, // 开发时关闭 Leader Election
	}

	// 如果指定了 namespace，只监听该 namespace
	if namespace != "" {
		options.Cache.DefaultNamespaces = map[string]cache.Config{namespace: {}}
		logger.Info("Watching single namespace", "namespace", namespace)
	} else {
		logger.Info("Watching all namespaces")
	}

	// 注册 core/v1 类型
	if err := corev1.AddToScheme(options.Scheme); err != nil {
		logger.Error(err, "Failed to add core/v1 to scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	// 注册 Reconciler
	if err := (&ConfigMapReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "Unable to create controller")
		os.Exit(1)
	}

	fmt.Println(`
╔══════════════════════════════════════════════════════════════╗
║           Simple ConfigMap-to-Secret Controller              ║
╠══════════════════════════════════════════════════════════════╣
║  监听带有 annotation 的 ConfigMap，自动同步到 Secret          ║
║                                                              ║
║  Annotation: simple-controller/sync-to-secret                ║
║                                                              ║
║  测试方法:                                                   ║
║  kubectl create configmap test-cm \                          ║
║    --from-literal=username=admin \                           ║
║    --from-literal=password=secret123                         ║
║                                                              ║
║  kubectl annotate configmap test-cm \                        ║
║    simple-controller/sync-to-secret=true                     ║
║                                                              ║
║  kubectl get secret test-cm-synced -o yaml                   ║
╚══════════════════════════════════════════════════════════════╝
`)

	logger.Info("Starting manager...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
