// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

/*
#include "aggregator_types.h"

void SubmitMetric(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheck(char *, char *, int, char **, char *, char *);
void SubmitEvent(char *, event_t *);
void SubmitHistogramBucket(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEvent(char *, char *, int, char *);

const aggregator_t aggregator = {
	SubmitMetric,
	SubmitServiceCheck,
	SubmitEvent,
	SubmitHistogramBucket,
	SubmitEventPlatformEvent,
};

void *get_aggregator() {
	return (void *)&aggregator;
}
*/
import "C"

import "unsafe"

var aggregator unsafe.Pointer

func init() {
	aggregator = C.get_aggregator()
}

// GetAggregator returns the structure containing every aggregator function callback
func GetAggregator() unsafe.Pointer {
	return aggregator
}
