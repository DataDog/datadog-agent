package serverless

import (
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Daemon is the communcation server for between the runtime and the serverless Agent.
// The name "daemon" is just in order to avoid serverless.StartServer ...
type Daemon struct {
	httpServer   *http.Server
	statsdServer *dogstatsd.Server
	stopCh       chan struct{}
	// Wait on this WaitGroup in controllers to be sure that the Daemon is ready.
	// (i.e. that the DogStatsD server is properly instanciated)
	ReadyWg *sync.WaitGroup
}

// SetStatsdServer sets the DogStatsD server instance running when it is ready.
func (d *Daemon) SetStatsdServer(statsdServer *dogstatsd.Server) {
	d.statsdServer = statsdServer
}

// StartDaemon starts an HTTP server to receive messages from the runtime.
// The DogStatsD server is provided when ready (slightly later), to have the
// hello route available as soon as possible. However, the HELLO route is blocking
// to have a way for the runtime function to know when the Serverless Agent is ready.
// If the Flush route is called before the statsd server has been set, a 503
// is returned by the HTTP route.
func StartDaemon(stopCh chan struct{}) *Daemon {
	mux := http.NewServeMux()

	daemon := &Daemon{
		statsdServer: nil,
		httpServer:   &http.Server{Addr: ":8124", Handler: mux},
		stopCh:       stopCh,
		ReadyWg:      &sync.WaitGroup{},
	}

	mux.Handle("/lambda/hello", &Hello{daemon})
	mux.Handle("/lambda/flush", &Flush{daemon})

	// this wait group will be blocking until the DogStatsD server has been instanciated
	daemon.ReadyWg.Add(1)

	// start the HTTP server used to communicate with the clients
	go func() {
		if err := daemon.httpServer.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()

	return daemon
}

// Hello implements the basic Hello route, creating a way for the runtime to
// know that the serverless agent is running.
// It is blocking until the DogStatsD daemon is ready.
type Hello struct {
	daemon *Daemon
}

// ServeHTTP - see type Hello comment.
func (h *Hello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Hello route.")
	// if the DogStatsD daemon isn't ready, wait for it.
	h.daemon.ReadyWg.Wait()
}

// Flush is the route to call to do an immediate flush on the serverless agent.
// Returns 503 if the DogStatsD is not ready yet, 200 otherwise.
type Flush struct {
	daemon *Daemon
}

// ServeHTTP - see type Flush comment.
func (f *Flush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Flush route.")

	// if the DogStatsD daemon isn't ready, wait for it.
	f.daemon.ReadyWg.Wait()

	if f.daemon.statsdServer == nil {
		w.WriteHeader(503)
		w.Write([]byte("DogStatsD server not ready"))
		return
	}
	// synchronous flush
	f.daemon.statsdServer.Flush(true)
}
