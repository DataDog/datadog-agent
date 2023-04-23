// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.RegisterMetadataAvail("Docker availability", diagnose)
}

// diagnose the docker availability on the system
func diagnose() error {
	_, err := GetDockerUtil()
	if err != nil {
		return fmt.Errorf("error connecting to docker: %w", err)
	}
	log.Info("successfully connected to docker")

	hostname, err := GetHostname(context.TODO())
	if err != nil {
		return fmt.Errorf("returned hostname %q with error: %w", hostname, err)
	}
	log.Infof("successfully got hostname %q from docker", hostname)

	return nil
}
