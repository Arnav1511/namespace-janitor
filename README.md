# namespace-janitor

A Kubernetes controller that automatically deletes namespaces after a
configurable time-to-live, to stop ephemeral environments accumulating
in shared clusters.

Built with [Kubebuilder](https://book.kubebuilder.io/) and
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).

## How it works

The controller watches all `Namespace` objects in the cluster. A namespace
opts in by carrying a TTL annotation:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: preview-1234
  annotations:
    janitor.arnavranjan.com/ttl: "2h"
```

On each reconcile the controller reads the namespace's creation timestamp,
adds the TTL, and compares against now. If the deadline has passed the
namespace is deleted. If it has not, the controller requeues itself for the
remaining duration rather than polling.

The requeue is an optimisation, not a correctness mechanism. The deadline is
derived entirely from the namespace's own `creationTimestamp` and annotation,
so the controller keeps no state of its own: if it restarts, the informer's
initial list reconciles every namespace again and each deadline is recomputed
from scratch. Nothing drifts and nothing is missed.

The TTL value is parsed with
[`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration), so `"90m"`,
`"2h"` and `"36h"` are all valid. Note that days are not a unit — use `"24h"`
rather than `"1d"`.

### What is ignored

A namespace is left alone when it:

- has no `janitor.arnavranjan.com/ttl` annotation,
- is already terminating,
- has a TTL that does not parse, or that is zero or negative, or
- is one of `default`, `kube-system`, `kube-public` or `kube-node-lease`.

A malformed TTL is logged and otherwise ignored rather than retried. It is a
user error, not a transient failure, so requeueing would retry the same bad
value under backoff indefinitely; correcting the annotation triggers a fresh
reconcile through the watch anyway.

The four protected namespaces are never deleted even when explicitly
annotated. The annotation is not a strong enough signal to justify that blast
radius — a mistyped `kubectl annotate` should not be able to take out the
control plane's workloads.

### RBAC

The controller only ever reads and deletes namespaces, so its ClusterRole
grants `get`, `list`, `watch` and `delete` on `namespaces` and nothing else.

## Tuning

The cache's `SyncPeriod` is left at controller-runtime's 10 hour default,
deliberately. The periodic resync is insurance against a bug that leaves an
object un-requeued; it is not a cache freshness mechanism, since it replays
the local cache as synthetic update events rather than re-listing from the
apiserver. Upstream's guidance is that if you want insurance against missed
events you should requeue rather than shorten the period — which is already
how this controller works. Shortening it would re-reconcile every namespace in
the cluster on a fixed interval, reintroducing exactly the polling the design
avoids.

Two related notes:

- Do not add `predicate.GenerationChangedPredicate` to the watch. Namespaces
  carry no `metadata.generation` at all, so the predicate would compare unset
  against unset, find no change, and filter out every update event — including
  the annotation edits this controller exists to react to, and the resync
  described above.
- Leader election is wired up but off by default. Turn it on before running
  more than one replica.

## Status

- [x] Project scaffold and controller registration
- [x] Reconcile loop: fetch namespace and log
- [x] TTL annotation parsing
- [x] Expiry check and deletion
- [x] Requeue on remaining TTL
- [x] envtest coverage

Possible next steps: emit a Kubernetes `Event` on a malformed annotation so
`kubectl describe ns` surfaces it, and export a metric for namespaces reaped.

## Running locally

Requires Go 1.26+, Docker, kind and kubectl.

```sh
kind create cluster --name janitor
make run
```

The controller runs as a local process against your current kubeconfig
context. There are no CRDs, so `make install` is not needed.

In a second terminal, create a namespace that expires in a minute:

```sh
kubectl create ns demo-expiring
kubectl annotate ns demo-expiring janitor.arnavranjan.com/ttl=1m
```

The controller logs the remaining lifetime, requeues, and deletes the
namespace once the minute is up. A namespace created without the annotation
is ignored indefinitely.

## Tests

```sh
make test
```

Unit tests run against [envtest](https://book.kubebuilder.io/reference/envtest),
which starts a real apiserver and etcd. Note that envtest runs no namespace
lifecycle controller, so a deleted namespace stays `Terminating` rather than
disappearing; the tests assert on the deletion timestamp accordingly.

## Licence

Apache 2.0
