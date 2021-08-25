// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const persistedStateFilePath = "/tmp/dd-lambda-extension-cache.json"

// shutdownDelay is the amount of time we wait before shutting down the HTTP server
// after we receive a Shutdown event. This allows time for the final log messages
// to arrive from the Logs API.
const shutdownDelay time.Duration = 1000 * time.Millisecond

// Daemon is the communcation server for between the runtime and the serverless Agent.
// The name "daemon" is just in order to avoid serverless.StartServer ...
type Daemon struct {
	httpServer *http.Server
	mux        *http.ServeMux

	MetricAgent *metrics.ServerlessMetricAgent

	TraceAgent *trace.ServerlessTraceAgent

	// lastInvocations stores last invocation times to be able to compute the
	// interval of invocation of the function.
	lastInvocations []time.Time

	// flushStrategy is the currently selected flush strategy, defaulting to the
	// the "flush at the end" naive strategy.
	flushStrategy flush.Strategy

	// useAdaptiveFlush is set to false when the flush strategy has been forced
	// through configuration.
	useAdaptiveFlush bool

	// clientLibReady indicates whether the datadog client library has initialised
	// and called the /hello route on the agent
	clientLibReady bool

	// stopped represents whether the Daemon has been stopped
	stopped bool

	// Wait on this WaitGroup to be sure that the daemon isn't doing any pending
	// work before finishing an invocation
	InvcWg *sync.WaitGroup

	ExtraTags *serverlessLog.Tags

	ExecutionContext *serverlessLog.ExecutionContext

	// finishInvocationOnce assert that FinishedInvocation will be called only once (at the end of the function OR after a timeout)
	// this should be reset before each invocation
	finishInvocationOnce sync.Once
}

// Hello implements the basic Hello route, creating a way for the Datadog Lambda Library
// to know that the serverless agent is running. It is blocking until the DogStatsD daemon is ready.
type Hello struct {
	daemon *Daemon
}

// ServeHTTP - see type Hello comment.
func (h *Hello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Hello route.")
	// if the DogStatsD daemon isn't ready, wait for it.
	h.daemon.SetClientReady(true)
}

// Flush is the route to call to do an immediate flush on the serverless agent.
// Returns 503 if the DogStatsD is not ready yet, 200 otherwise.
type Flush struct {
	daemon *Daemon
}

// ServeHTTP - see type Flush comment.
func (f *Flush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Flush route.")
	if !f.daemon.ShouldFlush(flush.Stopping, time.Now()) {
		log.Debug("The flush strategy", f.daemon.LogFlushStategy(), " has decided to not flush in moment:", flush.Stopping)
		f.daemon.FinishInvocation()
		return
	}

	log.Debug("The flush strategy", f.daemon.LogFlushStategy(), " has decided to flush in moment:", flush.Stopping)

	// if the DogStatsD daemon isn't ready, wait for it.
	if !f.daemon.MetricAgent.IsReady() {
		w.WriteHeader(503)
		w.Write([]byte("DogStatsD server not ready"))
		f.daemon.FinishInvocation()
		return
	}

	// note that I am not using the request context because I think that we don't
	// want the flush to be canceled if the client is closing the request.
	go func() {
		flushTimeout := config.Datadog.GetDuration("forwarder_timeout") * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		f.daemon.TriggerFlush(ctx, false)
		f.daemon.FinishInvocation()
		cancel()
	}()

}

// SetClientReady indicates that the client library has initialised and called the /hello route on the agent
func (d *Daemon) SetClientReady(isReady bool) {
	d.clientLibReady = isReady
}

// ShouldFlush indicated whether or a flush is needed
func (d *Daemon) ShouldFlush(moment flush.Moment, t time.Time) bool {
	return d.flushStrategy.ShouldFlush(moment, t)
}

// LogFlushStategy returns the flush stategy
func (d *Daemon) LogFlushStategy() string {
	return d.flushStrategy.String()
}

//SetupLogCollectionHandler configures the log collection route handler
func (d *Daemon) SetupLogCollectionHandler(route string, logsChan chan *logConfig.ChannelMessage, logsEnabled bool, enhancedMetricsEnabled bool) {
	d.mux.Handle(route, &serverlessLog.CollectionRouteInfo{
		ExtraTags:              d.ExtraTags,
		ExecutionContext:       d.ExecutionContext,
		LogChannel:             logsChan,
		MetricChannel:          d.MetricAgent.GetMetricChannel(),
		LogsEnabled:            logsEnabled,
		EnhancedMetricsEnabled: enhancedMetricsEnabled,
	})
}

// SetStatsdServer sets the DogStatsD server instance running when it is ready.
func (d *Daemon) SetStatsdServer(metricAgent *metrics.ServerlessMetricAgent) {
	d.MetricAgent = metricAgent
	d.MetricAgent.SetExtraTags(d.ExtraTags.Tags)
}

// SetTraceAgent sets the Agent instance for submitting traces
func (d *Daemon) SetTraceAgent(traceAgent *trace.ServerlessTraceAgent) {
	d.TraceAgent = traceAgent
}

// SetFlushStrategy sets the flush strategy to use.
func (d *Daemon) SetFlushStrategy(strategy flush.Strategy) {
	log.Debugf("Set flush strategy: %s (was: %s)", strategy.String(), d.LogFlushStategy())
	d.flushStrategy = strategy
}

// UseAdaptiveFlush sets whether we use the adaptive flush or not.
// Set it to false when the flush strategy has been forced through configuration.
func (d *Daemon) UseAdaptiveFlush(enabled bool) {
	d.useAdaptiveFlush = enabled
}

// TriggerFlush triggers a flush of the aggregated metrics, traces and logs.
// They are flushed concurrently.
// In some circumstances, it may switch to another flush strategy after the flush.
// isLastFlush indicates whether this is the last flush before the shutdown or not.
func (d *Daemon) TriggerFlush(ctx context.Context, isLastFlush bool) {
	// Increment the invocation wait group which tracks whether work is in progress for the daemon
	d.InvcWg.Add(1)
	defer d.InvcWg.Done()
	wg := sync.WaitGroup{}
	wg.Add(1)
	wg.Add(1)
	wg.Add(1)

	// metrics
	go func() {
		if d.MetricAgent != nil {
			d.MetricAgent.Flush()
		}
		wg.Done()
	}()

	// traces
	go func() {
		if d.TraceAgent != nil {
			d.TraceAgent.Get().FlushSync()
		}
		wg.Done()
	}()

	// logs
	go func() {
		logs.Flush(ctx)
		wg.Done()
	}()

	wg.Wait()
	log.Debug("Flush done")

	// After flushing, re-evaluate flush strategy (if applicable)
	if !isLastFlush {
		d.UpdateStrategy()
	}
}

// Stop causes the Daemon to gracefully shut down. After a delay, the HTTP server
// is shut down, data is flushed a final time, and then the agents are shut down.
func (d *Daemon) Stop() {
	// Can't shut down before starting
	// If the DogStatsD daemon isn't ready, wait for it.

	if d.stopped {
		log.Debug("Daemon.Stop() was called, but Daemon was already stopped")
		return
	}
	d.stopped = true

	// Wait for any remaining logs to arrive via the logs API before shutting down the HTTP server
	log.Debug("Waiting to shut down HTTP server")
	time.Sleep(shutdownDelay)

	log.Debug("Shutting down HTTP server")
	err := d.httpServer.Shutdown(context.Background())
	if err != nil {
		log.Error("Error shutting down HTTP server")
	}

	// Once the HTTP server is shut down, it is safe to shut down the agents
	// Otherwise, we might try to handle API calls after the agent has already been shut down
	d.TriggerFlush(context.Background(), true)

	log.Debug("Shutting down agents")

	if d.TraceAgent != nil {
		d.TraceAgent.Stop()
	}

	if d.MetricAgent != nil {
		d.MetricAgent.Stop()
	}
	logs.Stop()
	log.Debug("Serverless agent shutdown complete")
}

// StartDaemon starts an HTTP server to receive messages from the runtime.
// The DogStatsD server is provided when ready (slightly later), to have the
// hello route available as soon as possible. However, the HELLO route is blocking
// to have a way for the runtime function to know when the Serverless Agent is ready.
// If the Flush route is called before the statsd server has been set, a 503
// is returned by the HTTP route.
func StartDaemon(addr string) *Daemon {
	log.Debug("Starting daemon to receive messages from runtime...")
	mux := http.NewServeMux()
	daemon := &Daemon{
		httpServer:       &http.Server{Addr: addr, Handler: mux},
		mux:              mux,
		InvcWg:           &sync.WaitGroup{},
		lastInvocations:  make([]time.Time, 0),
		useAdaptiveFlush: true,
		clientLibReady:   false,
		flushStrategy:    &flush.AtTheEnd{},
		ExtraTags:        &serverlessLog.Tags{},
		ExecutionContext: &serverlessLog.ExecutionContext{},
	}
	log.Debug("Adaptive flush is enabled")

	mux.Handle("/lambda/hello", &Hello{daemon})
	mux.Handle("/lambda/flush", &Flush{daemon})

	// start the HTTP server used to communicate with the clients
	go func() {
		_ = daemon.httpServer.ListenAndServe()
	}()

	return daemon
}

// StartInvocation tells the daemon the invocation began
func (d *Daemon) StartInvocation() {
	d.finishInvocationOnce = sync.Once{}
	d.InvcWg.Add(1)
}

// FinishInvocation finishes the current invocation
func (d *Daemon) FinishInvocation() {
	d.finishInvocationOnce.Do(func() {
		d.InvcWg.Done()
	})
}

// WaitForDaemon waits until invocation finished any pending work
func (d *Daemon) WaitForDaemon() {
	if d.clientLibReady {
		d.InvcWg.Wait()
	}
}

// WaitUntilClientReady will wait until the client library has called the /hello route, or timeout
func (d *Daemon) WaitUntilClientReady(timeout time.Duration) bool {
	checkInterval := 10 * time.Millisecond
	for timeout > checkInterval {
		if d.clientLibReady {
			return true
		}
		<-time.After(checkInterval)
		timeout -= checkInterval
	}
	<-time.After(timeout)
	return d.clientLibReady
}

// ComputeGlobalTags extracts tags from the ARN, merges them with any user-defined tags and adds them to traces, logs and metrics
func (d *Daemon) ComputeGlobalTags(configTags []string) {
	if len(d.ExtraTags.Tags) == 0 {
		tagMap := tags.BuildTagMap(d.ExecutionContext.ARN, configTags)
		tagArray := tags.BuildTagsFromMap(tagMap)
		if d.MetricAgent != nil {
			d.MetricAgent.SetExtraTags(tagArray)
		}
		if d.TraceAgent != nil {
			d.TraceAgent.Get().SetGlobalTagsUnsafe(tags.BuildTracerTags(tagMap))
		}
		d.ExtraTags.Tags = tagArray
		source := serverlessLog.GetLambdaSource()
		if source != nil {
			source.Config.Tags = tagArray
		}
	}
}

// SetExecutionContext sets the current context to the daemon
func (d *Daemon) SetExecutionContext(arn string, requestID string) {
	d.ExecutionContext.ARN = arn
	d.ExecutionContext.LastRequestID = requestID
	if len(d.ExecutionContext.ColdstartRequestID) == 0 {
		d.ExecutionContext.Coldstart = true
		d.ExecutionContext.ColdstartRequestID = requestID
	} else {
		d.ExecutionContext.Coldstart = false
	}
}

// SaveCurrentExecutionContext stores the current context to a file
func (d *Daemon) SaveCurrentExecutionContext() error {
	file, err := json.Marshal(d.ExecutionContext)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(persistedStateFilePath, file, 0644)
	if err != nil {
		return err
	}
	return nil
}

// RestoreCurrentStateFromFile loads the current context from a file
func (d *Daemon) RestoreCurrentStateFromFile() error {
	file, err := ioutil.ReadFile(persistedStateFilePath)
	if err != nil {
		return err
	}
	var restoredExecutionContext serverlessLog.ExecutionContext
	err = json.Unmarshal(file, &restoredExecutionContext)
	if err != nil {
		return err
	}
	d.ExecutionContext.ARN = restoredExecutionContext.ARN
	d.ExecutionContext.LastRequestID = restoredExecutionContext.LastRequestID
	d.ExecutionContext.LastLogRequestID = restoredExecutionContext.LastLogRequestID
	d.ExecutionContext.ColdstartRequestID = restoredExecutionContext.ColdstartRequestID
	return nil
}
