/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ttlAnnotation opts a namespace in to expiry. Its value is a Go duration
// string, e.g. "2h" or "30m", measured from the namespace's creation time.
const ttlAnnotation = "janitor.arnavranjan.com/ttl"

// protectedNamespaces are never deleted, even when annotated. Deleting any of
// these would break the cluster, and the annotation is not a strong enough
// signal to justify that blast radius.
var protectedNamespaces = map[string]struct{}{
	"default":         {},
	"kube-system":     {},
	"kube-public":     {},
	"kube-node-lease": {},
}

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Now returns the current time. Left nil in production; tests set it to a
	// fixed clock so expiry and requeue durations are exactly assertable.
	Now func() time.Time
}

func (r *NamespaceReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;delete

// Reconcile deletes a namespace once its TTL annotation has elapsed, measured
// from the namespace's creation timestamp. Namespaces without the annotation
// are ignored. Namespaces that have not yet expired are requeued for exactly
// their remaining lifetime.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var ns corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Already on its way out; deleting again would be a no-op.
	if !ns.DeletionTimestamp.IsZero() || ns.Status.Phase == corev1.NamespaceTerminating {
		return ctrl.Result{}, nil
	}

	value, ok := ns.Annotations[ttlAnnotation]
	if !ok {
		return ctrl.Result{}, nil
	}

	if _, protected := protectedNamespaces[ns.Name]; protected {
		log.Info("refusing to expire protected namespace", "namespace", ns.Name, "ttl", value)
		return ctrl.Result{}, nil
	}

	// A malformed annotation is a user error, not a transient failure. Returning
	// an error would retry the same bad value forever under backoff, so log it
	// and stop; the watch will reconcile again when the annotation is corrected.
	ttl, err := time.ParseDuration(value)
	if err != nil {
		log.Error(err, "ignoring namespace: malformed TTL annotation",
			"namespace", ns.Name, "annotation", ttlAnnotation, "value", value)
		return ctrl.Result{}, nil
	}
	if ttl <= 0 {
		log.Info("ignoring namespace: TTL annotation must be positive",
			"namespace", ns.Name, "annotation", ttlAnnotation, "value", value)
		return ctrl.Result{}, nil
	}

	deadline := ns.CreationTimestamp.Add(ttl)
	if remaining := deadline.Sub(r.now()); remaining > 0 {
		log.Info("namespace not yet expired, requeueing",
			"namespace", ns.Name, "ttl", ttl, "remaining", remaining)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	log.Info("TTL expired, deleting namespace",
		"namespace", ns.Name, "ttl", ttl, "createdAt", ns.CreationTimestamp.Time)
	if err := r.Delete(ctx, &ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Named("namespace").
		Complete(r)
}
