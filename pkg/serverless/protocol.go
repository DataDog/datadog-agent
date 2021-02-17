package serverless

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// httpServerPort will be the default port used to run the HTTP server listening
// to calls from the client libraries and to logs from the AWS environment.
const httpServerPort int = 8124

const httpLogsCollectionRoute string = "/lambda/logs"

// Daemon is the communcation server for between the runtime and the serverless Agent.
// The name "daemon" is just in order to avoid serverless.StartServer ...
type Daemon struct {
	httpServer *http.Server
	mux        *http.ServeMux

	statsdServer *dogstatsd.Server

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
		httpServer:   &http.Server{Addr: fmt.Sprintf(":%d", httpServerPort), Handler: mux},
		mux:          mux,
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

// EnableLogsCollection is adding the HTTP route on which the HTTP server will receive
// logs from AWS.
// Returns the HTTP URL on which AWS should send the logs.
func (d *Daemon) EnableLogsCollection() (string, chan aws.LogMessage, error) {
	httpAddr := fmt.Sprintf("http://sandbox:%d%s", httpServerPort, httpLogsCollectionRoute)
	logsChan := make(chan aws.LogMessage)
	d.mux.Handle(httpLogsCollectionRoute, &LogsCollection{daemon: d, ch: logsChan})
	log.Debugf("Logs collection route has been initialized. Logs must be sent to %s", httpAddr)
	return httpAddr, logsChan, nil
}

// LogsCollection is the route on which the AWS environment is sending the logs
// for the extension to collect them. It is attached to the main HTTP server
// already receiving hits from the libraries client.
type LogsCollection struct {
	daemon *Daemon
	ch     chan aws.LogMessage
}

// ServeHTTP - see type LogsCollection comment.
func (l *LogsCollection) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	var messages []aws.LogMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		log.Error("Can't read log message")
		w.WriteHeader(400)
	} else {
		metricsChan := l.daemon.aggregator.GetBufferedMetricsWithTsChannel()
		functionARN := aws.GetARN()
		functionName := aws.FunctionNameFromARN()
		// FIXME(remy): could this be exported to properly get all tags?
		metricTags := []string{
			fmt.Sprintf("functionname:%s", functionName),
			fmt.Sprintf("function_arn:%s", functionARN),
		}
		for _, message := range messages {
			switch message.Type {
			case aws.LogTypePlatformStart:
				if len(message.ObjectRecord.RequestID) > 0 {
					aws.SetRequestID(message.ObjectRecord.RequestID)
				}
				l.ch <- message
			case aws.LogTypeFunction:
				if functionName != "" {
					generateEnhancedMetricsFromFunctionLog(message, metricTags, metricsChan)
				}
				l.ch <- message
			case aws.LogTypePlatformReport:
				if functionName != "" {
					generateEnhancedMetricsFromReportLog(message, metricTags, metricsChan)
				}
				l.ch <- message
			default:
				l.ch <- message
			}
		}
		w.WriteHeader(200)
	}
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
