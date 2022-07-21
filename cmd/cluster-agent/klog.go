// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// redirectLogger is used to redirect klog logs to datadog logs. klog is
// client-go's logger, logging to STDERR by default, which makes all severities
// into ERROR, along with the formatting just being off. To make the
// conversion, we set a redirectLogger as klog's output, and parse the severity
// and log message out of every log line.
// NOTE: on klog v2 this parsing is no longer necessary, as it allows us to use
// klog.SetLogger() instead of klog.SetOutputBySeverity(). unfortunately we
// still have some dependencies stuck on v1, so we keep the parsing.
type redirectLogger struct {
	stackDepth int
}

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
		log.InfoStackDepth(l.stackDepth, msg)
	case 'W':
		log.WarnStackDepth(l.stackDepth, msg)
	case 'E':
		log.ErrorStackDepth(l.stackDepth, msg)
	case 'F':
		log.CriticalStackDepth(l.stackDepth, msg)
	default:
		log.InfoStackDepth(l.stackDepth, msg)
	}

	return 0, nil
}

func init() {
	// klog takes configuration from command line flags or, like we're
	// doing here, a flagset passed as a parameter
	flagset := flag.NewFlagSet("", flag.ContinueOnError)

	// klog v1 and v2 use the same flags, so we only need to init it once,
	// with either of them (v2 chosen by roll of the dice)
	klogv2.InitFlags(flagset)

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
	klogv1.SetOutputBySeverity("INFO", redirectLogger{6})
	klogv2.SetOutputBySeverity("INFO", redirectLogger{7})
}
