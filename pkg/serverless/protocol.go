package serverless

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Daemon is the communcation server for between the runtime and the serverless Agent.
// The name "daemon" is just in order to avoid serverless.StartServer ...
type Daemon struct {
	httpServer *http.Server
	// http server used to collect AWS logs
	httpLogsServer *http.Server
	statsdServer   *dogstatsd.Server
	// aggregator used by the statsd server
	aggregator *aggregator.BufferedAggregator
	stopCh     chan struct{}
	// Wait on this WaitGroup in controllers to be sure that the Daemon is ready.
	// (i.e. that the DogStatsD server is properly instanciated)
	ReadyWg *sync.WaitGroup
}

// SetStatsdServer sets the DogStatsD server instance running when it is ready.
func (d *Daemon) SetStatsdServer(statsdServer *dogstatsd.Server) {
	d.statsdServer = statsdServer
}

// SetAggregator sets the aggregator used within the DogStatsD server.
// Use this aggregator `GetChannels()` or `GetBufferedChannels()` to send metrics
// directly to the aggregator, with caution.
func (d *Daemon) SetAggregator(aggregator *aggregator.BufferedAggregator) {
	d.aggregator = aggregator
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

// StartHttpLogsServer starts an HTTP server, receiving logs from the AWS platform.
// Returns the HTTP URL on which AWS should send the logs.
// FIXME(remy): that would be awesome to have this directly running within the initial HTTP daemon?
func (d *Daemon) StartHttpLogsServer(port int) (string, chan aws.LogMessage, error) {
	httpAddr := fmt.Sprintf("http://sandbox:%d", port)
	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)
	// http server receiving logs from the AWS Lambda environment

	logsChan := make(chan aws.LogMessage)

	go func() {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := ioutil.ReadAll(r.Body)
			defer r.Body.Close()
			var messages []aws.LogMessage
			if err := json.Unmarshal(data, &messages); err != nil {
				log.Error("Can't read log message")
				w.WriteHeader(400)
			} else {
				for _, message := range messages {
					switch message.Type {
					case aws.LogTypeExtension, aws.LogTypeFunction,
						aws.LogTypePlatformStart, aws.LogTypePlatformEnd:
						logsChan <- message
					case aws.LogTypePlatformReport:
						functionName := aws.FunctionNameFromARN()
						if functionName != "" {
							// report enhanced metrics using DogStatsD
							tags := []string{fmt.Sprintf("functionname:%s", functionName)} // FIXME(remy): could this be exported to properly get all tags?
							metricsChan := d.aggregator.GetBufferedMetricsWithTsChannel()
							metricsChan <- []metrics.MetricSample{metrics.MetricSample{
								Name:       "aws.lambda.enhanced.max_memory_used",
								Value:      float64(message.ObjectRecord.Metrics.MaxMemoryUsedMB),
								Mtype:      metrics.DistributionType,
								Tags:       tags,
								SampleRate: 1,
								Timestamp:  float64(message.Time.UnixNano()),
							}, metrics.MetricSample{
								Name:       "aws.lambda.enhanced.memorysize",
								Value:      float64(message.ObjectRecord.Metrics.MemorySizeMB),
								Mtype:      metrics.DistributionType,
								Tags:       tags,
								SampleRate: 1,
								Timestamp:  float64(message.Time.UnixNano()),
							}, metrics.MetricSample{
								Name:       "aws.lambda.enhanced.billed_duration",
								Value:      float64(message.ObjectRecord.Metrics.BilledDurationMs),
								Mtype:      metrics.DistributionType,
								Tags:       tags,
								SampleRate: 1,
								Timestamp:  float64(message.Time.UnixNano()),
							}, metrics.MetricSample{
								Name:       "aws.lambda.enhanced.duration",
								Value:      message.ObjectRecord.Metrics.DurationMs,
								Mtype:      metrics.DistributionType,
								Tags:       tags,
								SampleRate: 1,
								Timestamp:  float64(message.Time.UnixNano()),
							}, metrics.MetricSample{
								Name:       "aws.lambda.enhanced.init_duration",
								Value:      message.ObjectRecord.Metrics.InitDurationMs,
								Mtype:      metrics.DistributionType,
								Tags:       tags,
								SampleRate: 1,
								Timestamp:  float64(message.Time.UnixNano()),
							}}
						}
						logsChan <- message
					}
				}
				w.WriteHeader(200)
			}
		})
		s := &http.Server{
			Addr:         listenAddr,
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		log.Debug("Logs collection HTTP server starts")
		if err := s.ListenAndServe(); err != nil {
			log.Error("ListenAndServe:", err)
		}
	}()

	return httpAddr, logsChan, nil
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
	// synchronous flush of the logs agent
	// FIXME(remy): could the enhanced metrics be generated at this point? if not
	//              and they're already generated when REPORT is received on the http server,
	//              we could make this run in parallel with the statsd flush
	logs.Flush()
	// synchronous flush
	f.daemon.statsdServer.Flush(true)
	log.Debug("Sync flush done")
}
