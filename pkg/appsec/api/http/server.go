package http

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1"
)

type Server struct {
	*http.ServeMux
}


func newServeMux(c chan interface{}) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", newAPIVersionHandler(v0_1.NewServeMux(c)))
	return mux
}



func newAPIVersionHandler(v01 *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch v := r.Header.Get("X-Api-Version"); v {
		case "v0.1":
			v01.ServeHTTP(w, r)
		default:
			http.Error(w, "unexpected X-Api-Version value", http.StatusBadRequest)
		}
	})
}
