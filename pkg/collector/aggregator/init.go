// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aggregator exports functions to submit metrics, servive checks, events through RTLoader.
package aggregator

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"

#include <stdlib.h>

void SubmitMetricRtLoader(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheckRtLoader(char *, char *, int, char **, char *, char *);

void initAggregatorCallback() {
	set_aggregator_submit_metric_cb(SubmitMetricRtLoader);
	set_aggregator_submit_service_check_cb(SubmitServiceCheckRtLoader);
}
*/
import "C"

func init() {
	InitCallback()
}

// Initialize the callback for submitting metrics
func InitCallback() {
	C.initAggregatorCallback()
}
