package npschedulerimpl

import "github.com/DataDog/datadog-agent/pkg/util/log"

type tracerouteJob struct {
	destination string
	port        uint16
}

// Don't make it a method, to be overridden in tests
var worker = func(l *npSchedulerImpl, jobs <-chan tracerouteJob) {
	for {
		select {
		case <-l.stop:
			log.Debug("Stopping worker")
			return
		case job := <-jobs:
			log.Debugf("Handling Destination: %s", job.destination)
			l.runTraceroute(job)
		}
	}
}
