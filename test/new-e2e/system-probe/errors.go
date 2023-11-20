// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package systemProbe

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/system-probe/connector/metric"
	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/sethvargo/go-retry"
)

const (
	// bitmap of actions to take for an error
	retryStack = 0x1 // 0b01
	emitMetric = 0x2 // 0b10

	aria2cMissingStatusError = "error: wait: remote command exited without exit status or exit signal: running \" aria2c"
)

type handledError struct {
	errorStr string
	metric   string
	action   int
}

var errors = []handledError{
	// Retry if we failed to dial libvirt.
	// Libvirt daemon on the server occasionally drops the connection established
	// by the 'Provider'. If this happens we retry the stack to connect again.
	handledError{
		errorStr: "failed to dial libvirt",
		metric:   "failed-to-dial-libvirt",
		action:   retryStack | emitMetric,
	},
	handledError{
		errorStr: "InsufficientInstanceCapacity",
		metric:   "insufficient-capacity",
		action:   retryStack | emitMetric,
	},
	// Retry when ssh thinks aria2c exited without status. This may happen
	// due to network connectivity issues if ssh keepalive mecahnism fails.
	handledError{
		errorStr: aria2cMissingStatusError,
		metric:   "aria2c-exit-no-status",
		action:   retryStack | emitMetric,
	},
	handledError{
		errorStr: "timeout while waiting for state to become 'running'",
		metric:   "ec2-timeout-state-change",
		action:   emitMetric,
	},
}

func errorMetric(errType string) datadogV2.MetricPayload {
	tags := []string{
		fmt.Sprintf("error:%s", errType),
	}
	return datadogV2.MetricPayload{
		Series: []datadogV2.MetricSeries{
			{
				Metric: "datadog.e2e.system_probe.env-setup",
				Type:   datadogV2.METRICINTAKETYPE_COUNT.Ptr(),
				Points: []datadogV2.MetricPoint{
					{
						Timestamp: datadog.PtrInt64(time.Now().Unix()),
						Value:     datadog.PtrFloat64(1),
					},
				},
				Tags: tags,
			},
		},
	}
}

func handleScenarioFailure(err error) error {
	errStr := err.Error()
	for _, e := range errors {
		if !strings.Contains(errStr, e.errorStr) {
			continue
		}

		if (e.action & emitMetric) != 0 {
			submitError := metric.SubmitExecutionMetric(errorMetric(e.metric))
			log.Printf("failed to submit environment setup error metrics: %v\n", submitError)
		}

		if (e.action & retryStack) != 0 {
			log.Printf("environment setup error: %v. Retrying stack.\n", err)
			return retry.RetryableError(err)
		}

		break
	}

	log.Printf("environment setup error: %v. Failing stack.\n", err)
	return err
}
