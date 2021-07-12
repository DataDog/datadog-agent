// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("EC2 Metadata availability", diagnose)
}

// diagnose the ec2 metadata API availability
func diagnose() error {
	_, err := GetHostname(context.TODO())
	if err != nil {
		log.Error(err)
	}
	return err
}
