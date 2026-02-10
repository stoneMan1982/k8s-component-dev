# Kubernetes Informer Pipeline: Reflector, DeltaFIFO, Informer, Indexer, Workqueue, Worker

> This document explains the relationships among Reflector, DeltaFIFO, Informer, Indexer, workqueue, and worker in the Kubernetes controller pattern. It focuses on how events flow from the API Server to your controller logic.

## 1. Big Picture

Kubernetes controllers do not poll the API Server directly in tight loops. Instead, they use a shared cache and event pipeline:

1) Reflector watches the API Server and lists initial state.
2) Deltas are accumulated in a DeltaFIFO queue.
3) Informer pops deltas, updates a local cache (Indexer), and triggers handlers.
4) Handlers enqueue keys into a workqueue.
5) Workers pull keys from the queue and run business logic (Reconcile).

This design reduces API load, provides consistent local state, and enables backpressure and retry.

## 2. Components and Responsibilities

### Reflector

- Talks to the API Server using List + Watch.
- On startup, does a List to get a full snapshot and then switches to Watch for continuous updates.
- Produces a stream of events (Add/Update/Delete) for a single resource type.
- Does not store state; it only pushes deltas to the next stage.

### DeltaFIFO

- A queue that stores "deltas" (changes) instead of raw objects.
- Coalesces multiple updates for the same object key.
- Ensures ordering: all changes for a key are delivered in sequence.
- Supports resync: it can requeue existing objects to trigger periodic reconciliation.

### Informer

- The orchestrator that wires Reflector -> DeltaFIFO -> Indexer.
- Runs the Reflector in the background.
- Pops deltas from DeltaFIFO and updates the cache (Indexer).
- Calls registered event handlers (Add/Update/Delete) after cache is updated.
- Provides shared cache to multiple controllers, so they do not each watch the API Server.

### Indexer (Cache)

- A local in-memory store of objects keyed by namespace/name.
- Provides indexed lookups (e.g., by label, field, or custom index functions).
- Updated only by the Informer to keep a consistent view.
- Used by controllers to read state without hitting the API Server.

### Workqueue (Rate Limiting Queue)

- An in-process queue of work items (usually object keys).
- Supports rate limiting and retry with exponential backoff.
- Deduplicates keys so bursts of updates collapse into one work item.
- Helps protect your controller from overload.

### Worker

- A goroutine that pulls items from the workqueue.
- Calls your reconcile logic for each key.
- On error, requeues with rate limiting; on success, forgets the key.
- Multiple workers can run in parallel for higher throughput.

## 3. Relationship Diagram (Event Flow)

```mermaid
flowchart LR
    API[API Server] -->|List/Watch| R[Reflector]
    R --> D[DeltaFIFO]
    D --> I[Informer]
    I --> X[Indexer/Cache]
    I --> H[Event Handler]
    H --> Q[Workqueue]
    Q --> W[Worker(s)]
    W -->|Get from cache & reconcile| C[Controller Logic]
```

## 4. Key Behaviors and Guarantees

- **Cache First**: Informer updates the cache before calling handlers. This ensures handlers see the latest state.
- **Event Coalescing**: DeltaFIFO and workqueue both collapse multiple events for the same key.
- **Backpressure**: Workqueue rate limiting prevents fast event bursts from overwhelming workers.
- **Resync**: Informer can periodically requeue items to ensure convergence even if events are missed.
- **Eventual Consistency**: The cache is eventually consistent with the API Server; controllers should be idempotent.

## 5. Typical Controller Loop (Pseudocode)

```text
start informer
register event handlers

handler(onAdd/onUpdate/onDelete):
    enqueue key

worker loop:
    key = queue.get()
    obj = indexer.get(key)
    err = reconcile(obj)
    if err: queue.addRateLimited(key)
    else: queue.forget(key)
```

## 6. Why This Architecture Works Well

- **Scales**: Shared informers allow many controllers to share one watch per resource type.
- **Reduces API Load**: Most reads are served from the cache.
- **Resilient**: Requeue and resync handle transient errors and missed events.
- **Deterministic**: Reconcile is triggered by keys, not raw events, simplifying logic.

## 7. Common Pitfalls

- Writing non-idempotent reconcile logic (causes repeated updates).
- Relying on event order beyond the same key (not guaranteed across keys).
- Heavy API reads inside reconcile instead of using the cache.
- Not handling NotFound or stale cache reads gracefully.

---

If you want, I can add a second section that maps these components to controller-runtime internals (e.g., how it wires Informer and workqueue for your Reconcile).
