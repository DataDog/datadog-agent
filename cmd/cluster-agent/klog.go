// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows
// +build kubeapiserver

package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// redirectLogger is used to redirect klog logs to datadog logs. klog is
// client-go's logger, logging to STDERR by default, which makes all severities
// into ERROR, along with the formatting just being off. To make the
// conversion, we set a redirectLogger as klog's output, and parse the severity
// and log message out of every log line.
// NOTE: on klog v2, used by newer versions of client-go than the one we have
// right now, this parsing is no longer necessary, as it allows us to use
// klog.SetLogger() instead of klog.SetOutputBySeverity().
type redirectLogger struct{}

func (l redirectLogger) Write(b []byte) (int, error) {
	// klog log lines have the following format:
	//     Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...
	// so we parse L to decide in which level to log, and we try to find
	// the ']' character, to ignore anything up to that point, as we don't
	// care about the header outside of the log level.

	msg := string(b)

	i := strings.IndexByte(msg, ']')
	if i >= 0 {
		// if we find a ']', we ignore anything 2 positions from it
		// (itself, plus a blank space)
		msg = msg[i+2:]
	}

	switch b[0] {
	case 'I':
		log.InfoStackDepth(6, msg)
	case 'W':
		log.WarnStackDepth(6, msg)
	case 'E':
		log.ErrorStackDepth(6, msg)
	case 'F':
		log.CriticalStackDepth(6, msg)
	default:
		log.InfoStackDepth(6, msg)
	}

	return 0, nil
}

func init() {
	// klog takes configuration from command line flags or, like we're
	// doing here, a flagset passed as a parameter
	flagset := flag.NewFlagSet("", flag.ContinueOnError)
	klog.InitFlags(flagset)

	var err error

	// logtostderr is true by default, and when enabled promotes all logs
	// to ERROR when collected by the agent, so we disable it
	err = flagset.Set("logtostderr", strconv.FormatBool(false))
	if err != nil {
		panic(fmt.Sprintf("unable to set flag: %s", err))
	}

	// stderrthreshold is ERROR by default, but doing so would send
	// duplicated ERROR logs
	err = flagset.Set("stderrthreshold", "FATAL")
	if err != nil {
		panic(fmt.Sprintf("unable to set flag: %s", err))
	}

	err = flagset.Parse([]string{})
	if err != nil {
		panic(fmt.Sprintf("unable to parse flagset: %s", err))
	}

	// klog logs to an io.Writer on a certain severity, and every single
	// lower severity (so FATAL would log to FATAL, ERROR, WARN and INFO),
	// causing a lot of duplication, and that's why we parse the severity
	// out of the log instead of having an output for each severity. having
	// an output just for the lowest level captures the logs on all enabled
	// severities just once.
	klog.SetOutputBySeverity("INFO", redirectLogger{})
}
