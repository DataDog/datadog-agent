package api

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// dogstatsdProxyHandler returns a new HTTP handler which will proxy requests to the DogStatsD
// endpoint in the Core Agent over UDP.
func (r *HTTPReceiver) dogstatsdProxyHandler() http.Handler {
	if !r.conf.StatsdEnabled {
		log.Info("DogstatsD disabled in the Agent configuration")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		})
	}

	url := fmt.Sprintf("%s:%d", r.conf.StatsdHost, r.conf.StatsdPort)
	conn, err := net.Dial("udp", url)
	if err != nil {
		log.Error(err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer conn.Close()
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		conn.Write(b)
	})
}
