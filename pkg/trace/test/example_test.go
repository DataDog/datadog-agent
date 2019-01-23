package test

import (
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
)

// The below example shows a common use-case scenario for the runner.
func Example() {
	var runner Runner
	// Start the runner.
	if err := runner.Start(); err != nil {
		log.Fatal(err)
	}
	defer log.Fatal(runner.Shutdown(time.Second))

	// Run an agent with a given config.
	conf, err := ioutil.ReadFile("/opt/datadog-agent/etc/datadog.yaml")
	if err != nil {
		log.Fatal(err)
	}
	if err := runner.RunAgent(conf); err != nil {
		log.Fatal(err)
	}

	// Post a payload.
	payload := pb.Traces{
		pb.Trace{testutil.RandomSpan()},
		pb.Trace{testutil.RandomSpan()},
	}
	if err := runner.Post(payload); err != nil {
		log.Fatal(err)
	}

	// Assert the results.
	switch v := (<-runner.Out()).(type) {
	case pb.TracePayload:
		fmt.Println("OK traces: ", len(v.Traces))
	case agent.StatsPayload:
		fmt.Println("OK stats: ", len(v.Stats))
	}
}
