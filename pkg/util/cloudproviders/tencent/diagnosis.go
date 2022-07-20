// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tencent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"


func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\cloudproviders\tencent\diagnosis.go 14`)
	diagnosis.Register("Tencent Metadata availability", diagnose)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\cloudproviders\tencent\diagnosis.go 15`)
}

// diagnose the tencent cloud metadata API availability
func diagnose() error {
	_, err := GetInstanceID(context.TODO())
	return err
}