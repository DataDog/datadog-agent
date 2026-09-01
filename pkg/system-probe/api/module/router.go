// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"net/http"
	"runtime/pprof"
	"sync/atomic"
)

// Router provides a wrapper around http.ServeMux so routes can be re-registered.
type Router struct {
	router     *http.ServeMux
	labels     pprof.LabelSet
	registered atomic.Bool
}

// NewRouter returns a new Router
func NewRouter(namespace string, parent *http.ServeMux) *Router {
	subMux := http.NewServeMux()
	parent.Handle("/"+namespace+"/", http.StripPrefix("/"+namespace, subMux))
	r := &Router{
		router: subMux,
		labels: pprof.Labels("module", namespace),
	}
	r.registered.Store(true)
	return r
}

// HandleFunc registers a HandleFunc to automatically add pprof labels for the module.
// The pattern follows net/http.ServeMux conventions and may include an HTTP method prefix (e.g. "POST /path").
func (r *Router) HandleFunc(pattern string, handlerFn func(http.ResponseWriter, *http.Request)) {
	r.router.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		if !r.registered.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		pprof.Do(req.Context(), r.labels, func(ctx context.Context) {
			handlerFn(w, req.WithContext(ctx))
		})
	})
}

// Unregister removes the registered handler functions
func (r *Router) Unregister() {
	r.registered.Store(false)
}
