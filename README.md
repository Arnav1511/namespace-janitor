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

Namespaces without the annotation are ignored.

## Status

Work in progress.

- [x] Project scaffold and controller registration
- [ ] Reconcile loop: fetch namespace and log
- [ ] TTL annotation parsing
- [ ] Expiry check and deletion
- [ ] Requeue on remaining TTL
- [ ] envtest coverage

## Running locally

Requires Go 1.24+, Docker, kind and kubectl.

```sh
kind create cluster --name janitor
make run
```

The controller runs as a local process against your current kubeconfig
context. In a second terminal, create a namespace and watch the logs:

```sh
kubectl create ns test-1
```

## Licence

Apache 2.0
