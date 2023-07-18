// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package status

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/otlp"
)

// GetOTLPStatus parses the otlp pipeline and its collector info to be sent to the frontend
func GetOTLPStatus() map[string]interface{} {
	status := make(map[string]interface{})
	otlpIsEnabled := otlp.IsEnabled(config.Datadog)
	var otlpCollectorStatus otlp.CollectorStatus
	if otlpIsEnabled {
		otlpCollectorStatus = otlp.GetCollectorStatus(common.OTLP)
	} else {
		otlpCollectorStatus = otlp.CollectorStatus{Status: "Not running", ErrorMessage: ""}
	}

	status["otlpStatus"] = otlpIsEnabled
	status["otlpCollectorStatus"] = otlpCollectorStatus.Status
	status["otlpCollectorStatusErr"] = otlpCollectorStatus.ErrorMessage
	return status
}
