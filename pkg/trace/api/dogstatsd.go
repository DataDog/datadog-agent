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

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// TODO: Simplify this. We shouldn't need to open a new connection for every
		// request to the statsd endpoit. We should be able to just route the reverse
		// proxy to that endpoint.
		url := fmt.Sprintf("%s:%d", r.conf.StatsdHost, r.conf.StatsdPort)
		conn, err := net.Dial("udp", url)
		if err != nil {
			log.Error(err)
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		b, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		conn.Write(b)
	})
}
