// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gce

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("GCE Metadata availability", diagnose)
}

// diagnose the GCE metadata API availability
func diagnose() error {
	_, err := GetHostname(context.TODO())
	if err != nil {
		log.Error(err)
	}
	return err
}
