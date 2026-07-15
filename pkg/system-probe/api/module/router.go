// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"net/http"
	"runtime/pprof"
	"sync"
)

// Router provides a wrapper around http.ServeMux so routes can be re-registered.
// This is needed to support the module-restart feature.
type Router struct {
	mux            sync.Mutex
	handlerByRoute map[string]func(http.ResponseWriter, *http.Request)
	registered     map[string]bool
	router         *http.ServeMux
	labels         pprof.LabelSet
}

// NewRouter returns a new Router
func NewRouter(namespace string, parent *http.ServeMux) *Router {
	subMux := http.NewServeMux()
	parent.Handle("/"+namespace+"/", http.StripPrefix("/"+namespace, subMux))
	return &Router{
		handlerByRoute: make(map[string]func(http.ResponseWriter, *http.Request)),
		registered:     make(map[string]bool),
		router:         subMux,
		labels:         pprof.Labels("module", namespace),
	}
}

// HandleFunc registers a HandleFunc in such a way that routes can be registered multiple times.
// The pattern follows net/http.ServeMux conventions and may include an HTTP method prefix (e.g. "POST /path").
func (r *Router) HandleFunc(pattern string, responseWriter func(http.ResponseWriter, *http.Request)) {
	r.mux.Lock()
	_, registered := r.registered[pattern]
	r.registered[pattern] = true
	// overwrite the handler regardless if it was registered before
	r.handlerByRoute[pattern] = responseWriter
	r.mux.Unlock()

	if registered {
		// If this route was previously registered there is nothing left to do.
		return
	}

	r.router.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		r.mux.Lock()
		// obtain the current handler inline, which allows module restart
		handlerFn, ok := r.handlerByRoute[pattern]
		r.mux.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		pprof.Do(req.Context(), r.labels, func(_ context.Context) {
			handlerFn(w, req)
		})
	})
}

// Unregister removes the registered handler functions
func (r *Router) Unregister() {
	r.mux.Lock()
	defer r.mux.Unlock()
	clear(r.handlerByRoute)
}
