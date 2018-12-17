// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

	go j.heartbeat(ticker)

	for {
		// TODO: what should we do with error codes
		j.Wait()
		stopTimes[idx] = time.Now()
		oldestIdx := (idx + maxRestarts + 1) % maxRestarts

		if stopTimes[idx].Sub(stopTimes[oldestIdx]).Seconds() <= ival {
			log.Errorf("Too many JMXFetch restarts (%v) in time interval (%vs) - giving up")
			close(j.stopped)
			return
		}

		idx = (idx + 1) % maxRestarts

		select {
		case <-j.shutdown:
			close(j.stopped)
			return
		default:
			// restart
			log.Warnf("JMXFetch process had to be restarted.")
			j.Start(false)
		}
	}
}
