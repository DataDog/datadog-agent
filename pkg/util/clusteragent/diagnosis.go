// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"


func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\clusteragent\diagnosis.go 12`)
	diagnosis.Register("Cluster Agent availability", diagnose)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\clusteragent\diagnosis.go 13`)
}

func diagnose() error {
	_, err := GetClusterAgentClient()
	return err
}