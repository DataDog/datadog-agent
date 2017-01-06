package check

import (
	"net/http"

	"github.com/gorilla/mux"
)

// SetupHandlers adds the specific handlers for /check endpoints
func SetupHandlers(r *mux.Router) {
	r.HandleFunc("/", listChecks).Methods("GET")
	r.HandleFunc("/{name}", listChecks).Methods("GET", "DELETE")
	r.HandleFunc("/{name}/reload", reloadCheck).Methods("POST")
}

func reloadCheck(w http.ResponseWriter, r *http.Request) {

}

func listChecks(w http.ResponseWriter, r *http.Request) {

}
