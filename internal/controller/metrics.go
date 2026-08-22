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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Deliberately unlabelled. Namespace names are unbounded and short-lived, so
// using one as a label would grow the time series cardinality without bound.
var (
	namespacesDeleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "namespace_janitor_deleted_total",
		Help: "Number of namespaces deleted because their TTL elapsed.",
	})

	invalidTTLs = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "namespace_janitor_invalid_ttl_total",
		Help: "Number of reconciles that saw a malformed or non-positive TTL annotation.",
	})
)

func init() {
	metrics.Registry.MustRegister(namespacesDeleted, invalidTTLs)
}
