package module

import (
	"net/http"
	"sync"

	"github.com/gorilla/mux"
)

// Router provides a wrapper around mux.Router so routes can be re-registered
// This is needed to support the module-restart feature
type Router struct {
	mux            sync.Mutex
	handlerByRoute map[string]func(http.ResponseWriter, *http.Request)
	router         *mux.Router
}

// NewRouter returns a new Router
func NewRouter(mux *mux.Router) *Router {
	return &Router{
		handlerByRoute: make(map[string]func(http.ResponseWriter, *http.Request)),
		router:         mux,
	}
}

// HandleFunc registers a HandleFunc in such a way that routes can be registered multiple times
func (r *Router) HandleFunc(path string, responseWriter func(http.ResponseWriter, *http.Request)) *mux.Route {
	r.mux.Lock()
	_, registered := r.handlerByRoute[path]
	r.handlerByRoute[path] = responseWriter
	r.mux.Unlock()

	if registered {
		// If this route was previously registered there is nothing left to do.
		// The return value serves as a stub to support modules that are (re)registering routes
		// chaining calls like HandleFunc(path, handler).Method("POST")
		return new(mux.Route)
	}

	return r.router.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		r.mux.Lock()
		handlerFn := r.handlerByRoute[path]
		r.mux.Unlock()
		handlerFn(w, req)
	})
}
