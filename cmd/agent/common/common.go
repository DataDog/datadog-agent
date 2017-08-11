// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/kardianos/osext"
)

var (
	// AC is the global object orchestrating checks' loading and running
	AC *autodiscovery.AutoConfig

	// DSD is the global dogstastd instance
	DSD *dogstatsd.Server

	// MetadataScheduler is responsible to orchestrate metadata collection
	MetadataScheduler *metadata.Scheduler

	// Forwarder is the global forwarder instance
	Forwarder forwarder.Forwarder

	// Stopper is the channel used by other packages to ask for stopping the agent
	Stopper = make(chan bool)

	// utility variables
	_here, _ = osext.ExecutableFolder()
)
