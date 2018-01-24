// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package logs

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

// Start returns an error because logs-agent is not supported on windows yet
func Start() error {
	return fmt.Errorf("logs-agent is not supported on windows yet")
}

// Stop does nothing at the moment
func Stop() {}

// GetStatus returns is not running at the moment
func GetStatus() status.Status {
	return status.Status{IsRunning: false}
}
