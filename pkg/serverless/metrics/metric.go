package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ServerlessMetricAgent struct {
	DogStatDServer *dogstatsd.Server
	Aggregator     *aggregator.BufferedAggregator
}

func (c *ServerlessMetricAgent) Start(aggregatorInstance *aggregator.BufferedAggregator, waitingChan chan bool) {

	// prevents any UDP packets from being stuck in the buffer and not parsed during the current invocation
	// by setting this option to 1ms, all packets received will directly be sent to the parser
	config.Datadog.Set("dogstatsd_packet_buffer_flush_timeout", 1*time.Millisecond)

	statsd, err := dogstatsd.NewServer(aggregatorInstance, nil)
	if err != nil {
		// we're not reporting the error to AWS because we don't want the function
		// execution to be stopped. TODO(remy): discuss with AWS if there is way
		// of reporting non-critical init errors.
		// serverless.ReportInitError(serverlessID, serverless.FatalDogstatsdInit)
		log.Errorf("Unable to start the DogStatsD server: %s", err)
	}
	statsd.ServerlessMode = true // we're running in a serverless environment (will removed host field from samples)
	c.DogStatDServer = statsd
	c.Aggregator = aggregatorInstance

	waitingChan <- true
}

func (c *ServerlessMetricAgent) Get() *dogstatsd.Server {
	return c.DogStatDServer
}
