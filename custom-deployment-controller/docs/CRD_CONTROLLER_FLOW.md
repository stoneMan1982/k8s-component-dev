# CRD + Controller Development

This checklist walks from idea to a running CRD and controller. It is meant to be followed in order.

## 1. Define the Resource Model

- Clarify the problem: what should the custom resource control?
- Design `spec` (desired state) and `status` (observed state).
- Decide dependencies (Deployment, StatefulSet, Service, etc.).

## 2. Choose Group and Version

- Example: `apps.myorg.io/v1alpha1`.
- Plan how versions may evolve (alpha -> beta -> v1).

## 3. Write Go Types

- Create `CustomDeploymentSpec` and `CustomDeploymentStatus`.
- Create `CustomDeployment` and `CustomDeploymentList`.
- Add markers for code generation:
  - `+kubebuilder:object:root=true`
  - `+kubebuilder:subresource:status`

## 4. Generate DeepCopy

- Run controller-gen to generate `zz_generated.deepcopy.go`.
- Verify `DeepCopyObject()` exists for both types.

## 5. Register Types in Scheme

- Implement `AddToScheme` in your API package.
- Register both `CustomDeployment` and `CustomDeploymentList`.

## 6. Write the CRD YAML

- `metadata.name` is `<plural>.<group>`.
- Set `spec.group`, `spec.names`, `spec.scope`, `spec.versions`.
- Define `openAPIV3Schema` for `spec` and `status`.
- Enable `subresources.status`.

## 7. Implement Reconcile

Core loop (high level):

1) Get the custom resource.
2) Observe desired vs actual state.
3) Create or update child resources.
4) Update status.
5) Requeue on error.

## 8. Wire the Controller

- Create manager with scheme.
- Register controller with:
  - `For(&CustomDeployment{})`
  - `Owns(&Deployment{})` (or other child resources)
- Start the manager.

## 9. Local Testing

- `go run .` to start controller (uses kubeconfig).
- `kubectl apply -f crd.yaml`.
- `kubectl apply -f customdeployment.yaml`.
- Inspect results:
  - `kubectl get customdeployments -o yaml`
  - `kubectl get deploy <name> -o yaml`

## 10. Production Hardening

- Ensure reconcile is idempotent.
- Add finalizers if you manage external resources.
- Add predicates to reduce noisy events.
- Add RBAC rules if running in cluster.
- Add logging and metrics.
