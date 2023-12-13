// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package main

import (
	"flag"
	"fmt"
	"strconv"

	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	klogv1.SetOutputBySeverity("INFO", log.NewKlogRedirectLogger(6))
	klogv2.SetOutputBySeverity("INFO", log.NewKlogRedirectLogger(7))
}
