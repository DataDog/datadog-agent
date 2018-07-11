package api

import (
	"crypto/tls"
	stdLog "log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// Server manages the lifecycle of the HTTPS API server
type Server struct {
	m           sync.RWMutex
	tlsListener net.Listener
	https       *http.Server
	router      *mux.Router
	listening   bool
}

// NewServer instanciates a Server with the given parameters
func NewServer(listener net.Listener, tlsConfig *tls.Config, validators ...mux.MiddlewareFunc) *Server {
	s := &Server{
		router:    mux.NewRouter(),
		listening: false,
	}

	// Apply TLS over the listener
	s.tlsListener = tls.NewListener(listener, tlsConfig)

	// Initialise the router
	s.router.Use(validators...)

	// Initialise the https server
	s.https = &http.Server{
		Handler:      s.router,
		ErrorLog:     stdLog.New(&config.ErrorLogWriter{}, "", 0), // log errors to seelog
		TLSConfig:    tlsConfig,
		WriteTimeout: config.Datadog.GetDuration("server_timeout") * time.Second,
	}

	return s
}

// Router exposes the router to register endpoints
func (s *Server) Router() *mux.Router {
	return s.router
}

// SetWriteTimeout allows to overwrite the defalt timeout, set from the
// "server_timeout" viper option
func (s *Server) SetWriteTimeout(duration time.Duration) {
	s.m.Lock()
	defer s.m.Unlock()

	s.https.WriteTimeout = duration
}

// Start starts the serving goroutine, to be called once setup is finished
func (s *Server) Start() {
	s.m.Lock()
	defer s.m.Unlock()

	go s.https.Serve(s.tlsListener)
	s.listening = true
}

// Stop closes the connection and the server stops listening to new commands
func (s *Server) Stop() {
	s.m.Lock()
	defer s.m.Unlock()

	if s.listening {
		s.tlsListener.Close()
		s.listening = false
	}
}
