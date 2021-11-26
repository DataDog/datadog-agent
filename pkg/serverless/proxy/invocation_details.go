// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"time"
)

type invocationDetails struct {
	startTime          time.Time
	endTime            time.Time
	isError            bool
	invokeHeaders      map[string][]string
	invokeEventPayload string
}

func (i *invocationDetails) isComplete() bool {
	return !i.startTime.IsZero() && !i.endTime.IsZero() && i.invokeHeaders != nil && len(i.invokeEventPayload) > 0
}

func (i *invocationDetails) reset() {
	i.startTime = time.Time{}
	i.endTime = time.Time{}
	i.isError = false
	i.invokeHeaders = nil
	i.invokeEventPayload = ""
}
