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
	"strings"
	"sync"
	"time"

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
const shutdownDelay time.Duration = 1 * time.Second

// FlushTimeout is the amount of time to wait for a flush to complete.
const FlushTimeout time.Duration = 5 * time.Second

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

	// stopped represents whether the Daemon has been stopped
	stopped bool

	// RuntimeWg is used to keep track of whether the runtime is currently handling an invocation.
	// It should be reset when we start a new invocation, as we may start a new invocation before hearing that the last one finished.
	RuntimeWg *sync.WaitGroup

	// FlushWg is used to keep track of whether there is currently a flush in progress
	FlushWg *sync.WaitGroup

	ExtraTags *serverlessLog.Tags

	ExecutionContext *serverlessLog.ExecutionContext

	// TellDaemonRuntimeDoneOnce asserts that TellDaemonRuntimeDone will be called at most once per invocation (at the end of the function OR after a timeout)
	// this should be reset before each invocation
	TellDaemonRuntimeDoneOnce sync.Once

	// metricsFlushMutex ensures that only one metrics flush can be underway at a given time
	metricsFlushMutex sync.Mutex

	// tracesFlushMutex ensures that only one traces flush can be underway at a given time
	tracesFlushMutex sync.Mutex

	// logsFlushMutex ensures that only one logs flush can be underway at a given time
	logsFlushMutex sync.Mutex
}

// StartDaemon starts an HTTP server to receive messages from the runtime.
// The DogStatsD server is provided when ready (slightly later), to have the
// hello route available as soon as possible. However, the HELLO route is blocking
// to have a way for the runtime function to know when the Serverless Agent is ready.
func StartDaemon(addr string) *Daemon {
	log.Debug("Starting daemon to receive messages from runtime...")
	mux := http.NewServeMux()

	daemon := &Daemon{
		httpServer:        &http.Server{Addr: addr, Handler: mux},
		mux:               mux,
		RuntimeWg:         &sync.WaitGroup{},
		FlushWg:           &sync.WaitGroup{},
		lastInvocations:   make([]time.Time, 0),
		useAdaptiveFlush:  true,
		flushStrategy:     &flush.AtTheEnd{},
		ExtraTags:         &serverlessLog.Tags{},
		ExecutionContext:  &serverlessLog.ExecutionContext{},
		metricsFlushMutex: sync.Mutex{},
		tracesFlushMutex:  sync.Mutex{},
		logsFlushMutex:    sync.Mutex{},
	}

	mux.Handle("/lambda/hello", &Hello{daemon})
	mux.Handle("/lambda/flush", &Flush{daemon})

	// start the HTTP server used to communicate with the runtime and the Lambda platform
	go func() {
		_ = daemon.httpServer.ListenAndServe()
	}()

	return daemon
}

// Hello is a route called by the Lambda Library when it starts.
// It is no longer used, but the route is maintained for backwards compatibility.
type Hello struct {
	daemon *Daemon
}

// ServeHTTP - see type Hello comment.
func (h *Hello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Hello route.")
}

// Flush is a route called by the Lambda Library when the runtime is done handling an invocation.
// It is no longer used, but the route is maintained for backwards compatibility.
type Flush struct {
	daemon *Daemon
}

// ServeHTTP - see type Flush comment.
func (f *Flush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Flush route.")
}

// HandleRuntimeDone should be called when the runtime is done handling the current invocation. It will tell the daemon
// that the runtime is done, and may also flush telemetry.
func (d *Daemon) HandleRuntimeDone() {
	if !d.ShouldFlush(flush.Stopping, time.Now()) {
		log.Debugf("The flush strategy %s has decided to not flush at moment: %s", d.GetFlushStrategy(), flush.Stopping)
		d.TellDaemonRuntimeDone()
		return
	}

	log.Debugf("The flush strategy %s has decided to flush at moment: %s", d.GetFlushStrategy(), flush.Stopping)

	// if the DogStatsD daemon isn't ready, wait for it.
	if !d.MetricAgent.IsReady() {
		log.Debug("The metric agent wasn't ready, skipping flush.")
		d.TellDaemonRuntimeDone()
		return
	}

	go func() {
		d.TriggerFlush(false)
		d.TellDaemonRuntimeDone()
	}()
}

// ShouldFlush indicated whether or a flush is needed
func (d *Daemon) ShouldFlush(moment flush.Moment, t time.Time) bool {
	return d.flushStrategy.ShouldFlush(moment, t)
}

// GetFlushStrategy returns the flush stategy
func (d *Daemon) GetFlushStrategy() string {
	return d.flushStrategy.String()
}

// SetupLogCollectionHandler configures the log collection route handler
func (d *Daemon) SetupLogCollectionHandler(route string, logsChan chan *logConfig.ChannelMessage, logsEnabled bool, enhancedMetricsEnabled bool) {
	d.mux.Handle(route, &serverlessLog.LambdaLogsCollector{
		ExtraTags:              d.ExtraTags,
		ExecutionContext:       d.ExecutionContext,
		LogChannel:             logsChan,
		MetricChannel:          d.MetricAgent.GetMetricChannel(),
		LogsEnabled:            logsEnabled,
		EnhancedMetricsEnabled: enhancedMetricsEnabled,
		HandleRuntimeDone:      d.HandleRuntimeDone,
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
	log.Debugf("Set flush strategy: %s (was: %s)", strategy.String(), d.GetFlushStrategy())
	d.flushStrategy = strategy
}

// UseAdaptiveFlush sets whether we use the adaptive flush or not.
// Set it to false when the flush strategy has been forced through configuration.
func (d *Daemon) UseAdaptiveFlush(enabled bool) {
	d.useAdaptiveFlush = enabled
}

// TriggerFlush triggers a flush of the aggregated metrics, traces and logs.
// If the flush times out, the daemon will stop waiting for the flush to complete, but the
// flush may be continued on the next invocation.
// In some circumstances, it may switch to another flush strategy after the flush.
func (d *Daemon) TriggerFlush(isLastFlushBeforeShutdown bool) {
	d.FlushWg.Add(1)
	defer d.FlushWg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), FlushTimeout)

	wg := sync.WaitGroup{}
	wg.Add(3)

	go d.flushMetrics(&wg)
	go d.flushTraces(&wg)
	go d.flushLogs(ctx, &wg)

	timedOut := waitWithTimeout(&wg, FlushTimeout)
	if timedOut {
		log.Debug("Timed out while flushing, flush may be continued on next invocation")
	} else {
		log.Debug("Finished flushing")
	}
	cancel()

	if !isLastFlushBeforeShutdown {
		d.UpdateStrategy()
	}
}

// flushMetrics flushes aggregated metrics to the intake.
// It is protected by a mutex to ensure only one metrics flush can be in progress at any given time.
func (d *Daemon) flushMetrics(wg *sync.WaitGroup) {
	d.metricsFlushMutex.Lock()
	flushStartTime := time.Now().Unix()
	log.Debugf("Beginning metrics flush at time %d", flushStartTime)
	if d.MetricAgent != nil {
		d.MetricAgent.Flush()
	}
	log.Debugf("Finished metrics flush that was started at time %d", flushStartTime)
	wg.Done()
	d.metricsFlushMutex.Unlock()
}

// flushTraces flushes aggregated traces to the intake.
// It is protected by a mutex to ensure only one traces flush can be in progress at any given time.
func (d *Daemon) flushTraces(wg *sync.WaitGroup) {
	d.tracesFlushMutex.Lock()
	flushStartTime := time.Now().Unix()
	log.Debugf("Beginning traces flush at time %d", flushStartTime)
	if d.TraceAgent != nil && d.TraceAgent.Get() != nil {
		d.TraceAgent.Get().FlushSync()
	}
	log.Debugf("Finished traces flush that was started at time %d", flushStartTime)
	wg.Done()
	d.tracesFlushMutex.Unlock()
}

// flushLogs flushes aggregated logs to the intake.
// It is protected by a mutex to ensure only one logs flush can be in progress at any given time.
func (d *Daemon) flushLogs(ctx context.Context, wg *sync.WaitGroup) {
	d.logsFlushMutex.Lock()
	flushStartTime := time.Now().Unix()
	log.Debugf("Beginning logs flush at time %d", flushStartTime)
	logs.Flush(ctx)
	log.Debugf("Finished logs flush that was started at time %d", flushStartTime)
	wg.Done()
	d.logsFlushMutex.Unlock()
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
	d.TriggerFlush(true)

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

// TellDaemonRuntimeStarted tells the daemon that the runtime started handling an invocation
func (d *Daemon) TellDaemonRuntimeStarted() {
	// Reset the RuntimeWg on every new invocation.
	// We might receive a new invocation before we learn that the previous invocation has finished.
	d.RuntimeWg = &sync.WaitGroup{}
	d.TellDaemonRuntimeDoneOnce = sync.Once{}
	d.RuntimeWg.Add(1)
}

// TellDaemonRuntimeDone tells the daemon that the runtime finished handling an invocation
func (d *Daemon) TellDaemonRuntimeDone() {
	d.TellDaemonRuntimeDoneOnce.Do(func() {
		d.RuntimeWg.Done()
	})
}

// WaitForDaemon waits until the daemon has finished handling the current invocation
func (d *Daemon) WaitForDaemon() {
	// We always want to wait for any in-progress flush to complete
	d.FlushWg.Wait()

	// If we are flushing at the end of the invocation, we need to wait for the invocation itself to end
	// before we finish handling it. Otherwise, the daemon does not actually need to wait for the runtime to
	// complete the invocation before it is done.
	if d.flushStrategy.ShouldFlush(flush.Stopping, time.Now()) {
		d.RuntimeWg.Wait()
	}
}

// ComputeGlobalTags extracts tags from the ARN, merges them with any user-defined tags and adds them to traces, logs and metrics
func (d *Daemon) ComputeGlobalTags(configTags []string) {
	if len(d.ExtraTags.Tags) == 0 {
		tagMap := tags.BuildTagMap(d.ExecutionContext.ARN, configTags)
		tagArray := tags.BuildTagsFromMap(tagMap)
		if d.MetricAgent != nil {
			d.MetricAgent.SetExtraTags(tagArray)
		}
		d.setTraceTags(tagMap)
		d.ExtraTags.Tags = tagArray
		source := serverlessLog.GetLambdaSource()
		if source != nil {
			source.Config.Tags = tagArray
		}
	}
}

// setTraceTags tries to set extra tags to the Trace agent.
// setTraceTags returns a boolean which indicate whether or not the operation succeed for testing purpose.
func (d *Daemon) setTraceTags(tagMap map[string]string) bool {
	if d.TraceAgent != nil && d.TraceAgent.Get() != nil {
		d.TraceAgent.Get().SetGlobalTagsUnsafe(tags.BuildTracerTags(tagMap))
		return true
	}
	return false
}

// SetExecutionContext sets the current context to the daemon
func (d *Daemon) SetExecutionContext(arn string, requestID string) {
	d.ExecutionContext.ARN = strings.ToLower(arn)
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
	d.ExecutionContext.StartTime = restoredExecutionContext.StartTime
	return nil
}
