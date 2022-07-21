// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package alibaba

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\cloudproviders\alibaba\diagnosis.go 14`)
	diagnosis.Register("Alibaba Metadata availability", diagnose)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\cloudproviders\alibaba\diagnosis.go 15`)
}

// diagnose the alibaba metadata API availability
func diagnose() error {
	_, err := GetHostAliases(context.TODO())
	return err
}
