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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// envtest runs only the apiserver and etcd, with no namespace lifecycle
// controller, so a deleted namespace stays in Terminating forever rather than
// disappearing. Deletion is therefore asserted via the deletion timestamp, and
// every spec uses a fresh namespace name.
var nsCounter int

// createNamespace creates a namespace and returns it as stored by the
// apiserver, which is where CreationTimestamp comes from.
func createNamespace(annotations map[string]string) *corev1.Namespace {
	GinkgoHelper()

	nsCounter++
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("janitor-test-%d", nsCounter),
			Annotations: annotations,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	Expect(ns.CreationTimestamp.IsZero()).To(BeFalse())

	return ns
}

// reconcileAt runs a single reconcile with the clock pinned to now.
func reconcileAt(name string, now time.Time) (ctrl.Result, error) {
	GinkgoHelper()

	r := &NamespaceReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
		Now:    func() time.Time { return now },
	}
	return r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
}

func getNamespace(name string) *corev1.Namespace {
	GinkgoHelper()

	var ns corev1.Namespace
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, &ns)).To(Succeed())
	return &ns
}

type ttlCase struct {
	annotations map[string]string
	// deleteFirst puts the namespace into Terminating before reconciling.
	deleteFirst bool
	// elapsed is how far past the namespace's creation the clock is set.
	elapsed time.Duration
	// wantRequeue is the exact expected Result.RequeueAfter; 0 means no requeue.
	wantRequeue time.Duration
	// wantDeleted is whether the namespace should be terminating afterwards.
	wantDeleted bool
}

var _ = Describe("Namespace Controller", func() {
	DescribeTable("reconciling a namespace",
		func(tc ttlCase) {
			ns := createNamespace(tc.annotations)
			if tc.deleteFirst {
				Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
			}

			result, err := reconcileAt(ns.Name, ns.CreationTimestamp.Add(tc.elapsed))

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(tc.wantRequeue))
			Expect(getNamespace(ns.Name).DeletionTimestamp.IsZero()).To(Equal(!tc.wantDeleted))
		},

		Entry("deletes a namespace whose TTL has elapsed", ttlCase{
			annotations: map[string]string{ttlAnnotation: "1h"},
			elapsed:     90 * time.Minute,
			wantDeleted: true,
		}),
		Entry("deletes a namespace exactly on its deadline", ttlCase{
			annotations: map[string]string{ttlAnnotation: "1h"},
			elapsed:     time.Hour,
			wantDeleted: true,
		}),
		Entry("requeues for the remaining TTL when not yet expired", ttlCase{
			annotations: map[string]string{ttlAnnotation: "2h"},
			elapsed:     30 * time.Minute,
			wantRequeue: 90 * time.Minute,
		}),
		Entry("ignores a namespace with no TTL annotation", ttlCase{
			annotations: nil,
			elapsed:     30 * 24 * time.Hour,
		}),
		Entry("ignores a namespace with an unrelated annotation", ttlCase{
			annotations: map[string]string{"example.com/owner": "arnav"},
			elapsed:     30 * 24 * time.Hour,
		}),
		Entry("ignores a namespace with a malformed TTL annotation", ttlCase{
			annotations: map[string]string{ttlAnnotation: "two hours"},
			elapsed:     30 * 24 * time.Hour,
		}),
		Entry("ignores a namespace with a non-positive TTL annotation", ttlCase{
			annotations: map[string]string{ttlAnnotation: "-5m"},
			elapsed:     30 * 24 * time.Hour,
		}),
		// Without the terminating guard this would requeue for the remaining 90m,
		// so a zero requeue is what proves the guard fired.
		Entry("ignores a namespace that is already terminating", ttlCase{
			annotations: map[string]string{ttlAnnotation: "2h"},
			deleteFirst: true,
			elapsed:     30 * time.Minute,
			wantDeleted: true,
		}),
	)

	It("refuses to delete a protected namespace even when it has expired", func() {
		const name = "kube-system"

		protected := getNamespace(name)
		if protected.Annotations == nil {
			protected.Annotations = map[string]string{}
		}
		protected.Annotations[ttlAnnotation] = "1s"
		Expect(k8sClient.Update(ctx, protected)).To(Succeed())
		DeferCleanup(func() {
			current := getNamespace(name)
			delete(current.Annotations, ttlAnnotation)
			Expect(k8sClient.Update(ctx, current)).To(Succeed())
		})

		result, err := reconcileAt(name, time.Now().Add(24*time.Hour))

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(getNamespace(name).DeletionTimestamp.IsZero()).To(BeTrue())
	})

	It("ignores a namespace that no longer exists", func() {
		result, err := reconcileAt("janitor-test-does-not-exist", time.Now())

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})
})
