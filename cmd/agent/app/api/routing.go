package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

func getRouter() *mux.Router {
	// root HTTP router
	r := mux.NewRouter()

	// IPC REST API server

	// go_expvar server
	r.Handle("/debug/vars", http.DefaultServeMux)

	return r
}
