package http

import (
	stdlog "log"
	"net/http"
	"time"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1_0"
	"github.com/DataDog/datadog-agent/pkg/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Server struct {
	server *http.Server
}

// NewServer creates a new HTTP server serving the AppSec HTTP API.
func NewServer(cfg *config.Config, c agenttypes.RawJSONEventsChan) Server {
	// HTTP server read/write timeouts
	timeout := 5 * time.Second
	// HTTP server address
	addr := cfg.HTTPAPIListenAddr
	// Error logger limited to 5 messages every 10 seconds
	errorLogger := stdlog.New(logutil.NewThrottled(5, 10*time.Second), "appsec http server: ", 0)

	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		ErrorLog:     errorLogger,
		Handler:      NewServeMux(c),
	}

	return Server{
		server: srv,
	}
}

func (s *Server) Start() {
	// Start the HTTP server
	go func() {
		defer watchdog.LogOnPanic()
		log.Infof("Listening for appsec events at http://%s", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			osutil.Exitf("Error starting the http server: %v", err)
		}
	}()
}

func NewServeMux(c agenttypes.RawJSONEventsChan) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", newAPIVersionHandler(v0_1_0.NewServeMux(c)))
	return mux
}

func newAPIVersionHandler(v0_1_0 *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch v := r.Header.Get("X-Api-Version"); v {
		case "v0.1.0":
			v0_1_0.ServeHTTP(w, r)
		default:
			http.Error(w, "unexpected X-Api-Version value", http.StatusBadRequest)
		}
	})
}
