// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkEndPrefix = "Done running check"

	// How long is the first series of check runs we want to log
	initialCheckLoggingSeriesLimit uint64 = 5

	// Key in the config which defines how often should we print the logging
	// messages after the initial logging series
	loggingFrequencyConfigKey = "logging_frequency"
)

// CheckLogger is the object responsible for logging the information generated
// during the running of the check
type CheckLogger struct {
	Check                     check.Check
	shouldLog, lastVerboseLog bool
}

// CheckStarted is used to log that the check is about to run
func (cl *CheckLogger) CheckStarted() {
	if cl.shouldLog, cl.lastVerboseLog = shouldLogCheck(cl.Check.ID()); cl.shouldLog {
		log.Infoc("Running check...", "check", cl.Check)
		return
	}

	log.Debugc("Running check...", "check", cl.Check)
}

// CheckFinished is used to log that the check has completed
func (cl *CheckLogger) CheckFinished() {
	message := checkEndPrefix

	if cl.shouldLog {
		if cl.lastVerboseLog {
			message += fmt.Sprintf(
				", next runs will be logged every %v runs",
				config.Datadog.GetInt64(loggingFrequencyConfigKey),
			)
		}

		log.Infoc(message, "check", cl.Check)
	} else {
		log.Debugc(message, "check", cl.Check)
	}

	if cl.Check.Interval() == 0 {
		log.Infoc("Check's one time execution has finished", "check", cl.Check)
	}
}

// Error is used to log an error that occurred during the invocation of the check
func (cl *CheckLogger) Error(checkErr error) {
	log.Errorc(fmt.Sprintf("Error running check: %s", checkErr), "check", cl.Check)
}

// Debug is used to log a message for a check that may be useful in debugging
func (cl *CheckLogger) Debug(message string) {
	log.Debugc(message, "check", cl.Check)
}

// shouldLogCheck returns if we should log the check start/stop message with higher
// verbosity and if this is the end of the initial series of check log statements
func shouldLogCheck(id check.ID) (bool, bool) {
	loggingFrequency := uint64(config.Datadog.GetInt64(loggingFrequencyConfigKey))

	// If this is the first time we see the check, log it
	stats, idFound := expvars.CheckStats(id)
	if !idFound {
		// We always log the first run message
		return true, false
	}

	// We print a special message when we change logging frequency
	lastVerboseLog := stats.TotalRuns == initialCheckLoggingSeriesLimit

	// `currentRun` is `stats.TotalRuns` + 1 as the first run would be 0.
	currentRun := stats.TotalRuns + 1
	// We log the first `initialCheckLoggingSeriesLimit` times, then every `loggingFrequency` times
	if currentRun <= initialCheckLoggingSeriesLimit ||
		currentRun%loggingFrequency == 0 {

		return true, lastVerboseLog
	}

	return false, lastVerboseLog
}
