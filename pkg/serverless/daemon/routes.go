package daemon

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Hello is a route called by the Datadog Lambda Library when it starts.
// It is used to detect the Datadog Lambda Library in the environment.
type Hello struct {
	daemon *Daemon
}

func (h *Hello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Hello route.")
	h.daemon.LambdaLibraryDetected = true
}

// Flush is a route called by the Datadog Lambda Library when the runtime is done handling an invocation.
// It is no longer used, but the route is maintained for backwards compatibility.
type Flush struct {
	daemon *Daemon
}

func (f *Flush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Flush route.")
}

// StartInvocation is a route that can be called at the beginning of an invocation to enable
// the invocation lifecyle feature without the use of a proxy.
type StartInvocation struct {
	daemon *Daemon
}

func (s *StartInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var startDetails invocationlifecycle.InvocationStartDetails

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Could not read StartInvocation request body")
	}

	json.Unmarshal(reqBody, &startDetails)

	log.Debug("Hit on the serverless.StartInvocation route.")
	log.Debug(startDetails)
}

// EndInvocation is a route that can be called at the end of an invocation to enable
// the invocation lifecyle feature without the use of a proxy.
type EndInvocation struct {
	daemon *Daemon
}

func (e *EndInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var endDetails invocationlifecycle.InvocationEndDetails

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Could not read StartInvocation request body")
	}

	json.Unmarshal(reqBody, &endDetails)

	log.Debug("Hit on the serverless.EndInvocation route.")
	log.Debug(endDetails)
}
