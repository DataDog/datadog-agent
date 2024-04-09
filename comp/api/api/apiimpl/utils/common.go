package utils

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func SetJSONError(w http.ResponseWriter, err error, errorCode int) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
}

// GetConnection returns the connection for the request
func GetConnection(r *http.Request) net.Conn {
	return r.Context().Value(grpc.ConnContextKey).(net.Conn)
}
