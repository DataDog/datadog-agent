// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/logs"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	traceAgent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// httpServerPort will be the default port used to run the HTTP server listening
// to calls from the client libraries and to logs from the AWS environment.
const httpServerPort int = 8124

const httpLogsCollectionRoute string = "/lambda/logs"

// shutdownDelay is the amount of time we wait before shutting down the HTTP server
// after we receive a Shutdown event. This allows time for the final log messages
// to arrive from the Logs API.
const shutdownDelay time.Duration = 1 * time.Second

// Daemon is the communcation server for between the runtime and the serverless Agent.
// The name "daemon" is just in order to avoid serverless.StartServer ...
type Daemon struct {
	httpServer *http.Server
	mux        *http.ServeMux

	statsdServer *dogstatsd.Server

	traceAgent     *traceAgent.Agent
	stopTraceAgent context.CancelFunc

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

	// aggregator used by the statsd server
	aggregator *aggregator.BufferedAggregator

	// stopped represents whether the Daemon has been stopped
	stopped bool
	// Wait on this WaitGroup in controllers to be sure that the Daemon is ready.
	// (i.e. that the DogStatsD server is properly instantiated)
	ReadyWg *sync.WaitGroup

	// Wait on this WaitGroup to be sure that the daemon isn't doing any pending
	// work before finishing an invocation
	InvcWg *sync.WaitGroup
}

// SetStatsdServer sets the DogStatsD server instance running when it is ready.
func (d *Daemon) SetStatsdServer(statsdServer *dogstatsd.Server) {
	d.statsdServer = statsdServer
}

// SetTraceAgent sets the Agent instance for submitting traces
func (d *Daemon) SetTraceAgent(traceAgent *traceAgent.Agent) {
	d.traceAgent = traceAgent
}

// SetAggregator sets the aggregator used within the DogStatsD server.
// Use this aggregator `GetChannels()` or `GetBufferedChannels()` to send metrics
// directly to the aggregator, with caution.
func (d *Daemon) SetAggregator(aggregator *aggregator.BufferedAggregator) {
	d.aggregator = aggregator
}

// SetFlushStrategy sets the flush strategy to use.
func (d *Daemon) SetFlushStrategy(strategy flush.Strategy) {
	log.Debugf("Set flush strategy: %s (was: %s)", strategy.String(), d.flushStrategy.String())
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
		if d.statsdServer != nil {
			d.statsdServer.Flush()
		}
		wg.Done()
	}()

	// traces
	go func() {
		if d.traceAgent != nil {
			d.traceAgent.FlushSync()
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
	d.ReadyWg.Wait()

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

	err = aws.PersistCurrentStateToFile()
	if err != nil {
		log.Error("Unable to persist current state to file while shutting down")
	}

	// Once the HTTP server is shut down, it is safe to shut down the agents
	// Otherwise, we might try to handle API calls after the agent has already been shut down
	d.TriggerFlush(context.Background(), true)

	log.Debug("Shutting down agents")
	d.stopTraceAgent()
	if d.statsdServer != nil {
		d.statsdServer.Stop()
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
func StartDaemon(stopTraceAgent context.CancelFunc) *Daemon {
	log.Debug("Starting daemon to receive messages from runtime...")
	mux := http.NewServeMux()

	daemon := &Daemon{
		statsdServer:     nil,
		httpServer:       &http.Server{Addr: fmt.Sprintf(":%d", httpServerPort), Handler: mux},
		stopTraceAgent:   stopTraceAgent,
		mux:              mux,
		ReadyWg:          &sync.WaitGroup{},
		InvcWg:           &sync.WaitGroup{},
		lastInvocations:  make([]time.Time, 0),
		useAdaptiveFlush: true,
		clientLibReady:   false,
		flushStrategy:    &flush.AtTheEnd{},
	}

	log.Debug("Adaptive flush is enabled")

	mux.Handle("/lambda/hello", &Hello{daemon})
	mux.Handle("/lambda/flush", &Flush{daemon})

	// this wait group will be blocking until the DogStatsD server has been instantiated
	daemon.ReadyWg.Add(1)

	// start the HTTP server used to communicate with the clients
	go func() {
		if err := daemon.httpServer.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()

	return daemon
}

// EnableLogsCollection is adding the HTTP route on which the HTTP server will receive
// logs from AWS.
// Returns the HTTP URL on which AWS should send the logs.
func (d *Daemon) EnableLogsCollection() (string, chan *logConfig.ChannelMessage, error) {
	httpAddr := fmt.Sprintf("http://sandbox:%d%s", httpServerPort, httpLogsCollectionRoute)
	logsChan := make(chan *logConfig.ChannelMessage)
	d.mux.Handle(httpLogsCollectionRoute, &LogsCollection{daemon: d, ch: logsChan})
	log.Debugf("Logs collection route has been initialized. Logs must be sent to %s", httpAddr)
	return httpAddr, logsChan, nil
}

// StartInvocation tells the daemon the invocation began
func (d *Daemon) StartInvocation() {
	d.InvcWg.Add(1)
}

// FinishInvocation finishes the current invocation
func (d *Daemon) FinishInvocation() {
	d.InvcWg.Done()
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

// LogsCollection is the route on which the AWS environment is sending the logs
// for the extension to collect them. It is attached to the main HTTP server
// already receiving hits from the libraries client.
type LogsCollection struct {
	daemon *Daemon
	ch     chan *logConfig.ChannelMessage
}

// ServeHTTP - see type LogsCollection comment.
func (l *LogsCollection) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// If the DogStatsD daemon isn't ready, wait for it.
	l.daemon.ReadyWg.Wait()

	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	messages, err := aws.ParseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		processLogMessages(l, messages)
		w.WriteHeader(200)
	}
}

func processLogMessages(l *LogsCollection, messages []aws.LogMessage) {
	metricsChan := l.daemon.aggregator.GetBufferedMetricsWithTsChannel()
	metricTags := getTagsForEnhancedMetrics()
	logsEnabled := config.Datadog.GetBool("serverless.logs_enabled")
	enhancedMetricsEnabled := config.Datadog.GetBool("enhanced_metrics")
	arn := aws.GetARN()
	lastRequestID := aws.GetRequestID()
	functionName := aws.FunctionNameFromARN()
	for _, message := range messages {
		processMessage(message, arn, lastRequestID, functionName, enhancedMetricsEnabled, metricTags, metricsChan)
		// We always collect and process logs for the purpose of extracting enhanced metrics.
		// However, if logs are not enabled, we do not send them to the intake.
		if logsEnabled {
			logMessage := logConfig.NewChannelMessageFromLambda([]byte(message.StringRecord), message.Time, arn, lastRequestID, functionName)
			l.ch <- logMessage
		}
	}
}

// processMessage performs logic about metrics and tags on the message
func processMessage(message aws.LogMessage, arn string, lastRequestID string, functionName string, computeEnhancedMetrics bool, metricTags []string, metricsChan chan []metrics.MetricSample) {
	// Do not send logs or metrics if we can't associate them with an ARN or Request ID
	// First, if the log has a Request ID, set the global Request ID variable
	if message.Type == aws.LogTypePlatformStart {
		if len(message.ObjectRecord.RequestID) > 0 {
			aws.SetRequestID(message.ObjectRecord.RequestID)
			lastRequestID = message.ObjectRecord.RequestID
		}
	}

	if !aws.ShouldProcessLog(arn, lastRequestID, message) {
		return
	}

	if computeEnhancedMetrics {
		generateEnhancedMetrics(message, metricTags, metricsChan)
	}

	switch message.Type {
	case aws.LogTypePlatformReport:
		aws.SetColdStart(false)
	case aws.LogTypePlatformLogsDropped:
		log.Debug("Logs were dropped by the AWS Lambda Logs API")
	}
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
	h.daemon.clientLibReady = true
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

	if !f.daemon.flushStrategy.ShouldFlush(flush.Stopping, time.Now()) {
		log.Debug("The flush strategy", f.daemon.flushStrategy, " has decided to not flush in moment:", flush.Stopping)
		f.daemon.FinishInvocation()
		return
	}

	log.Debug("The flush strategy", f.daemon.flushStrategy, " has decided to flush in moment:", flush.Stopping)

	// if the DogStatsD daemon isn't ready, wait for it.
	f.daemon.ReadyWg.Wait()
	if f.daemon.statsdServer == nil {
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
