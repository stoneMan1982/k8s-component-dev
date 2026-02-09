# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

A Kubernetes Controller that watches ConfigMaps with the annotation `simple-controller/sync-to-secret` and automatically syncs their data to corresponding Secrets (named `<configmap-name>-synced`).

Built with Go and controller-runtime (sigs.k8s.io/controller-runtime).

## Build and Run Commands

```powershell
# Download dependencies
go mod tidy

# Run controller (watches all namespaces)
go run main.go

# Run controller (single namespace)
go run main.go -namespace=default

# Build executable
go build -o controller.exe main.go

# Run with debug logging
go run main.go -zap-log-level=debug

# Run tests
go test -v ./...
```

## Testing the Controller

Requires a running Kubernetes cluster (`kubectl cluster-info` to verify).

```powershell
# Apply test ConfigMap (triggers Secret creation)
kubectl apply -f test-configmap.yaml

# Verify Secret was created
kubectl get secret my-app-config-synced -o yaml

# Test update sync
kubectl patch configmap my-app-config -p '{"data":{"NEW_KEY":"new-value"}}'

# Test cascade delete (Secret deleted when ConfigMap deleted)
kubectl delete configmap my-app-config
```

## Architecture

**Single-file controller** (`main.go`) with these components:

1. **ConfigMapReconciler** - Implements `Reconcile()` for the watch-reconcile loop:
   - Fetches ConfigMap by namespaced name
   - Checks for `simple-controller/sync-to-secret` annotation
   - Creates/updates Secret with `-synced` suffix
   - Sets OwnerReference for cascade deletion

2. **Manager setup** - Configures controller-runtime manager with:
   - Optional namespace filtering via `-namespace` flag
   - Metrics endpoint on `:8080`
   - Zap logger with dev mode

**Key pattern**: The controller uses `ctrl.SetControllerReference()` to establish owner relationships, enabling Kubernetes garbage collection to automatically delete Secrets when their source ConfigMaps are deleted.

## Debugging

See `DEBUG_GUIDE.md` for detailed debugging instructions including:
- Log levels (`-zap-log-level=debug`)
- Delve debugger setup
- VS Code launch configuration
- Common troubleshooting patterns
