// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/input/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewScanner returns a new container scanner,
// by default it returns a docker scanner unless it could not be set up properly ,
// otherwise returns a kubernetes scanner.
func NewScanner(sources *config.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry) (restart.Restartable, error) {
	var scanner restart.Restartable
	var err error
	// attempt to initialize a docker scanner
	scanner, err = docker.NewScanner(sources, pipelineProvider, registry)
	if err == nil {
		return scanner, nil
	}
	log.Warnf("Could not setup the docker scanner, falling back to the kubernetes one: %v", err)
	// attempt to initialize a kubernetes scanner
	scanner, err = kubernetes.NewScanner(sources)
	if err == nil {
		return scanner, nil
	}
	log.Warnf("Could not setup the kubernetes scanner: %v", err)
	return nil, err
}
