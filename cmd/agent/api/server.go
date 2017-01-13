/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"fmt"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/agent/api/agent"
	"github.com/DataDog/datadog-agent/cmd/agent/api/check"
	"github.com/gorilla/mux"
)

var (
	listener net.Listener
)

// StartServer creates the router and starts the HTTP server
func StartServer() {
	// create the root HTTP router
	r := mux.NewRouter()

	// IPC REST API server
	agent.SetupHandlers(r.PathPrefix("/agent").Subrouter())
	check.SetupHandlers(r.PathPrefix("/check").Subrouter())

	// get the transport we're going to use under HTTP
	var err error
	listener, err = getListener()
	if err != nil {
		// we use the listener to handle commands for the Agent, there's
		// no way we can recover from this error
		panic(fmt.Sprintf("Unable to create the api server: %v", err))
	}

	go http.Serve(listener, r)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	listener.Close()
}
