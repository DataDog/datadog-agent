package daemon

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const localTestEnvVar = "DD_LOCAL_TEST"

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
	if len(os.Getenv(localTestEnvVar)) > 0 {
		// used only for testing purpose as the Logs API is not supported by the Lambda Emulator
		// thus we canot get the REPORT log line telling that the invocation is finished
		f.daemon.HandleRuntimeDone()
	}
}

// StartInvocation is a route that can be called at the beginning of an invocation to enable
// the invocation lifecyle feature without the use of the proxy.
type StartInvocation struct {
	daemon *Daemon
}

func (s *StartInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.StartInvocation route.")
	startTime := time.Now()
	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Could not read StartInvocation request body")
		http.Error(w, "Could not read StartInvocation request body", 400)
		return
	}
	startDetails := &invocationlifecycle.InvocationStartDetails{
		StartTime:             startTime,
		InvokeEventRawPayload: string(reqBody),
	}
	s.daemon.InvocationProcessor.OnInvokeStart(startDetails)
}

// EndInvocation is a route that can be called at the end of an invocation to enable
// the invocation lifecycle feature without the use of the proxy.
type EndInvocation struct {
	daemon *Daemon
}

func (e *EndInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.EndInvocation route.")
	endTime := time.Now()
	ecs := e.daemon.ExecutionContext.GetCurrentState()
	responseBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		err := log.Error("Could not read EndInvocation request body")
		http.Error(w, err.Error(), 400)
		return
	}
	var endDetails = invocationlifecycle.InvocationEndDetails{
		EndTime:            endTime,
		IsError:            r.Header.Get(invocationlifecycle.InvocationErrorHeader) == "true",
		RequestID:          ecs.LastRequestID,
		ResponseRawPayload: responseBody,
	}
	e.daemon.InvocationProcessor.OnInvokeEnd(&endDetails)
}

// TraceContext is a route called by tracer so it can retrieve the tracing context
type TraceContext struct {
}

func (tc *TraceContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.TraceContext route.")
	w.Header().Set(invocationlifecycle.TraceIDHeader, fmt.Sprintf("%v", invocationlifecycle.TraceID()))
	w.Header().Set(invocationlifecycle.SpanIDHeader, fmt.Sprintf("%v", invocationlifecycle.SpanID()))
}
