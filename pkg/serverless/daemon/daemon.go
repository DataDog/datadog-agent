// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"net/http"
	"sync"
	"time"

	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/otlp"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ShutdownDelay is the amount of time we wait before shutting down the HTTP server
// after we receive a Shutdown event. This allows time for the final log messages
// to arrive from the Logs API.
var ShutdownDelay = 200 * time.Millisecond

// FlushTimeout is the amount of time to wait for a flush to complete.
const FlushTimeout time.Duration = 5 * time.Second

// Daemon is the communication server between the runtime and the serverless agent and coordinates the flushing of telemetry.
type Daemon struct {
	httpServer *http.Server
	mux        *http.ServeMux

	MetricAgent *metrics.ServerlessMetricAgent

	LogsAgent logsAgent.ServerlessLogsAgent

	TraceAgent *trace.ServerlessTraceAgent

	ColdStartCreator *trace.ColdStartSpanCreator

	OTLPAgent *otlp.ServerlessOTLPAgent

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

	// LambdaLibraryDetected represents whether the Datadog Lambda Library was detected in the environment
	LambdaLibraryDetected bool

	// runtimeStateMutex is used to ensure that modifying the state of the runtime is thread-safe
	runtimeStateMutex sync.Mutex

	// RuntimeWg is used to keep track of whether the runtime is currently handling an invocation.
	// It should be reset when we start a new invocation, as we may start a new invocation before hearing that the last one finished.
	RuntimeWg *sync.WaitGroup

	// FlushLock is used to keep track of whether there is currently a flush in progress
	FlushLock sync.Mutex

	ExtraTags *serverlessLog.Tags

	// ExecutionContext stores the context of the current invocation
	ExecutionContext *executioncontext.ExecutionContext

	// TellDaemonRuntimeDoneOnce asserts that TellDaemonRuntimeDone will be called at most once per invocation (at the end of the function OR after a timeout).
	// We store a pointer to a sync.Once, which should be reset to a new pointer at the beginning of each invocation.
	// Note that overwriting the actual underlying sync.Once is not thread safe,
	// so we must use a pointer here to create a new sync.Once without overwriting the old one when resetting.
	TellDaemonRuntimeDoneOnce *sync.Once

	// metricsFlushMutex ensures that only one metrics flush can be underway at a given time
	metricsFlushMutex sync.Mutex

	// tracesFlushMutex ensures that only one traces flush can be underway at a given time
	tracesFlushMutex sync.Mutex

	// logsFlushMutex ensures that only one logs flush can be underway at a given time
	logsFlushMutex sync.Mutex

	// InvocationProcessor is used to handle lifecycle events, either using the proxy or the lifecycle API
	InvocationProcessor invocationlifecycle.InvocationProcessor

	logCollector *serverlessLog.LambdaLogsCollector
}

// StartDaemon starts an HTTP server to receive messages from the runtime and coordinate
// the flushing of telemetry.
func StartDaemon(addr string) *Daemon {
	log.Debug("Starting daemon to receive messages from runtime...")
	mux := http.NewServeMux()

	daemon := &Daemon{
		httpServer:        &http.Server{Addr: addr, Handler: mux},
		mux:               mux,
		RuntimeWg:         &sync.WaitGroup{},
		FlushLock:         sync.Mutex{},
		lastInvocations:   make([]time.Time, 0),
		useAdaptiveFlush:  true,
		flushStrategy:     &flush.AtTheEnd{},
		ExtraTags:         &serverlessLog.Tags{},
		ExecutionContext:  &executioncontext.ExecutionContext{},
		metricsFlushMutex: sync.Mutex{},
		tracesFlushMutex:  sync.Mutex{},
		logsFlushMutex:    sync.Mutex{},
	}

	mux.Handle("/lambda/hello", wrapOtlpError(&Hello{daemon}))
	mux.Handle("/lambda/flush", &Flush{daemon})
	mux.Handle("/lambda/start-invocation", wrapOtlpError(&StartInvocation{daemon}))
	mux.Handle("/lambda/end-invocation", wrapOtlpError(&EndInvocation{daemon}))
	mux.Handle("/trace-context", &TraceContext{daemon})

	// start the HTTP server used to communicate with the runtime and the Lambda platform
	go func() {
		_ = daemon.httpServer.ListenAndServe()
	}()

	return daemon
}

func wrapOtlpError(handle http.Handler) http.Handler {
	if otlp.IsEnabled() {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The Datadog tracer should not be used when OTLP is enabled.
			// Doing so can lead to double creation of traces and over billing.
			log.Error("Datadog tracing layer detected when OTLP is enabled!  These features are mutually exclusive.")
			handle.ServeHTTP(w, r)
		})
	}
	return handle
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
	if d.MetricAgent != nil && !d.MetricAgent.IsReady() {
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
func (d *Daemon) SetupLogCollectionHandler(route string, logsChan chan *logConfig.ChannelMessage, logsEnabled bool, enhancedMetricsEnabled bool, lambdaInitMetricChan chan<- *serverlessLog.LambdaInitMetric) {

	d.logCollector = serverlessLog.NewLambdaLogCollector(logsChan,
		d.MetricAgent.Demux, d.ExtraTags, logsEnabled, enhancedMetricsEnabled, d.ExecutionContext, d.HandleRuntimeDone, lambdaInitMetricChan)
	server := serverlessLog.NewLambdaLogsAPIServer(d.logCollector.In)

	d.mux.Handle(route, &server)
}

// SetStatsdServer sets the DogStatsD server instance running when it is ready.
func (d *Daemon) SetStatsdServer(metricAgent *metrics.ServerlessMetricAgent) {
	d.MetricAgent = metricAgent
	d.MetricAgent.SetExtraTags(d.ExtraTags.Tags)
}

// SetLogsAgent sets the logs agent instance running when it is ready.
func (d *Daemon) SetLogsAgent(logsAgent logsAgent.ServerlessLogsAgent) {
	d.LogsAgent = logsAgent
}

// SetTraceAgent sets the Agent instance for submitting traces
func (d *Daemon) SetTraceAgent(traceAgent *trace.ServerlessTraceAgent) {
	d.TraceAgent = traceAgent
}

func (d *Daemon) SetOTLPAgent(otlpAgent *otlp.ServerlessOTLPAgent) {
	d.OTLPAgent = otlpAgent
}

func (d *Daemon) SetColdStartSpanCreator(creator *trace.ColdStartSpanCreator) {
	d.ColdStartCreator = creator
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
	d.FlushLock.Lock()
	defer d.FlushLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), FlushTimeout)

	wg := sync.WaitGroup{}
	wg.Add(3)

	go d.flushMetrics(&wg)
	go d.flushTraces(&wg)
	go d.flushLogs(ctx, &wg)

	timedOut := waitWithTimeout(&wg, FlushTimeout)
	if timedOut {
		log.Debug("Timed out while flushing")
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
	if d.LogsAgent != nil {
		d.LogsAgent.Flush(ctx)
	}
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
	time.Sleep(ShutdownDelay)

	log.Debug("Shutting down HTTP server")
	err := d.httpServer.Shutdown(context.Background())
	if err != nil {
		log.Error("Error shutting down HTTP server")
	}

	if d.logCollector != nil {
		d.logCollector.Shutdown()
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

	if d.ColdStartCreator != nil {
		d.ColdStartCreator.Stop()
	}

	if d.OTLPAgent != nil {
		d.OTLPAgent.Stop()
	}

	if d.LogsAgent != nil {
		d.LogsAgent.Stop()
	}
	log.Debug("Serverless agent shutdown complete")
}

// TellDaemonRuntimeStarted tells the daemon that the runtime started handling an invocation
func (d *Daemon) TellDaemonRuntimeStarted() {
	// Reset the RuntimeWg on every new invocation.
	// We might receive a new invocation before we learn that the previous invocation has finished.
	d.runtimeStateMutex.Lock()
	defer d.runtimeStateMutex.Unlock()
	d.RuntimeWg = &sync.WaitGroup{}
	d.TellDaemonRuntimeDoneOnce = &sync.Once{}
	d.RuntimeWg.Add(1)
}

// TellDaemonRuntimeDone tells the daemon that the runtime finished handling an invocation
func (d *Daemon) TellDaemonRuntimeDone() {
	d.runtimeStateMutex.Lock()
	defer d.runtimeStateMutex.Unlock()
	// It's possible that we have a lambda function from a previous invocation sending a finished
	// log line to the agent, and it's possible that this happens before the current invocation is
	// received, in which case TellDaemonRuntimeDoneOnce is nil. We add this check in to ensure that
	// if this is the case, it won't crash the extension. This should be safe, since the code that modifies
	// the Once is locked by a mutex.
	if d.TellDaemonRuntimeDoneOnce == nil {
		return
	}
	d.TellDaemonRuntimeDoneOnce.Do(func() {
		d.RuntimeWg.Done()
	})
}

// WaitForDaemon waits until the daemon has finished handling the current invocation
func (d *Daemon) WaitForDaemon() {
	// We always want to wait for any in-progress flush to complete
	d.FlushLock.Lock()
	d.FlushLock.Unlock() //nolint:staticcheck

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
		ecs := d.ExecutionContext.GetCurrentState()
		tagMap := tags.BuildTagMap(ecs.ARN, configTags)
		d.ExecutionContext.UpdateRuntime(tagMap[tags.RuntimeKey])
		tagArray := tags.BuildTagsFromMap(tagMap)
		if d.MetricAgent != nil {
			d.MetricAgent.SetExtraTags(tagArray)
		}
		d.setTraceTags(tagMap)

		d.ExtraTags.Tags = tagArray
		serverlessLog.SetLogsTags(tagArray)
	}
}

// StartLogCollection begins processing the logs we have already received from the Lambda Logs API.
// This should be called after an ARN and RequestId is available. Can safely be called multiple times.
func (d *Daemon) StartLogCollection() {
	d.logCollector.Start()
}

// setTraceTags tries to set extra tags to the Trace agent.
// setTraceTags returns a boolean which indicate whether or not the operation succeed for testing purpose.
func (d *Daemon) setTraceTags(tagMap map[string]string) bool {
	if d.TraceAgent != nil && d.TraceAgent.Get() != nil {
		d.TraceAgent.SetTags(tags.BuildTracerTags(tagMap))
		return true
	}
	return false
}
