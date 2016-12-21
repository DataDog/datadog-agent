/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.

Current available endpoints:
	* TODO
	* TODO
*/
package api

import "net/http"

// BUG(massi): make this configurable through datadog.conf

// StartServer creates the router and starts the HTTP server
func StartServer() {
	r := getRouter()
	go http.ListenAndServe("localhost:5000", r)
}
