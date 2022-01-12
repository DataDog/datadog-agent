package daemon

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
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
// the invocation lifecyle feature without the use of the proxy.
type StartInvocation struct {
	daemon *Daemon
}

func (s *StartInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.StartInvocation route.")

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Could not read StartInvocation request body")
	}

	var startDetails invocationlifecycle.InvocationStartDetails
	json.Unmarshal(reqBody, &startDetails)

	s.daemon.LifecycleProcessor.OnInvokeStart(&startDetails)
}

// EndInvocation is a route that can be called at the end of an invocation to enable
// the invocation lifecyle feature without the use of the proxy.
type EndInvocation struct {
	daemon *Daemon
}

func (e *EndInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.EndInvocation route.")

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Could not read StartInvocation request body")
	}

	var endDetails invocationlifecycle.InvocationEndDetails
	json.Unmarshal(reqBody, &endDetails)

	e.daemon.LifecycleProcessor.OnInvokeEnd(&endDetails)
}

// TraceContext is a route called by tracer so it can retrieve the tracing context
type TraceContext struct {
}

// TraceContext - see type TraceContext comment.
func (tc *TraceContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.TraceContext route.")

	// TODO use traceID and spanID from the generated span
	traceID := uint64(rand.Uint32())<<32 + uint64(rand.Uint32())
	spanID := uint64(rand.Uint32())<<32 + uint64(rand.Uint32())

	w.Header().Set("x-datadog-trace-id", fmt.Sprintf("%v", traceID))
	w.Header().Set("x-datadog-span-id", fmt.Sprintf("%v", spanID))
	w.WriteHeader(200)
}
