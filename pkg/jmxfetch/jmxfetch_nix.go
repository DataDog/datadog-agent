// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build jmx
// +build !windows

package jmxfetch

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (j *JMXFetch) Monitor() {
	idx := 0
	maxRestarts := config.Datadog.GetInt("jmx_max_restarts")
	ival := float64(config.Datadog.GetInt("jmx_restart_interval"))
	stopTimes := make([]time.Time, maxRestarts)
	ticker := time.NewTicker(500 * time.Millisecond)

	defer ticker.Stop()
	defer close(j.stopped)

	go j.heartbeat(ticker)

	for {
		err := j.Wait()
		if err == nil {
			log.Infof("JMXFetch stopped and exited sanely.")
			break
		}

		stopTimes[idx] = time.Now()
		oldestIdx := (idx + maxRestarts + 1) % maxRestarts

		// Please note that the zero value for `time.Time` is `0001-01-01 00:00:00 +0000 UTC`
		// therefore for the first iteration (the initial launch attempt), the interval will
		// always be biger than ival (jmx_restart_interval). In fact, this sub operation with
		// stopTimes here will only start yielding values potentially <= ival _after_ the first
		// maxRestarts attempts, which is fine and consistent.
		if stopTimes[idx].Sub(stopTimes[oldestIdx]).Seconds() <= ival {
			log.Errorf("Too many JMXFetch restarts (%v) in time interval (%vs) - giving up", maxRestarts, ival)
			return
		}

		idx = (idx + 1) % maxRestarts

		select {
		case <-j.shutdown:
			return
		default:
			// restart
			log.Warnf("JMXFetch process had to be restarted.")
			j.Start(false)
		}
	}

	<-j.shutdown
}
