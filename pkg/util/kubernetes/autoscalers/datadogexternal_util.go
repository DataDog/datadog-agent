// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"strconv"
	"time"

	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

type minRemainingRequests struct {
	val            int
	timestamp      time.Time
	expiryDuration time.Duration
}

func newMinRemainingRequests(expiryDuration time.Duration) minRemainingRequests {
	return minRemainingRequests{
		val:            -1,
		timestamp:      time.Now(),
		expiryDuration: expiryDuration,
	}
}

func (mrr *minRemainingRequests) update(newVal string) {
	newValFloat, err := strconv.Atoi(newVal)

	if err != nil {
		return
	}

	isSet := mrr.val >= 0
	hasExpired := time.Since(mrr.timestamp) > mrr.expiryDuration

	if mrr.val >= newValFloat || !isSet || hasExpired {
		mrr.val = newValFloat
		mrr.timestamp = time.Now()
		rateLimitsRemainingMin.Set(float64(mrr.val), queryEndpoint, le.JoinLeaderValue)
	}
}
