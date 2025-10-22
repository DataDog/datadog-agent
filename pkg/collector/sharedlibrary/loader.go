// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// SharedLibraryCheckLoaderName is the name of the Shared Library loader
const SharedLibraryCheckLoaderName string = "sharedlibrary"

// SharedLibraryCheckLoader is a specific loader for checks living in this package
//
//nolint:revive
type SharedLibraryCheckLoader struct {
	logReceiver option.Option[integrations.Component]
	loader      libraryLoader
}

// NewSharedLibraryCheckLoader creates the checks loader
func NewSharedLibraryCheckLoader(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filter workloadfilter.Component, loader libraryLoader) (*SharedLibraryCheckLoader, error) {
	initializeCheckContext(senderManager, logReceiver, tagger, filter)
	return &SharedLibraryCheckLoader{
		logReceiver: logReceiver,
		loader:      loader,
	}, nil
}

// Name returns Shared Library loader name
func (*SharedLibraryCheckLoader) Name() string {
	return SharedLibraryCheckLoaderName
}

func (sl *SharedLibraryCheckLoader) String() string {
	return "Shared Library Loader"
}

// Load returns a Shared Library check
func (sl *SharedLibraryCheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, _instanceIndex int) (check.Check, error) {
	// load the library and get pointers to it and its 'Run' symbol through the library loader
	libHandles, err := sl.loader.Load(config.Name)
	if err != nil {
		return nil, err
	}

	// Create the check
	c, err := NewSharedLibraryCheck(senderManager, config.Name, sl.loader, libHandles)
	if err != nil {
		return c, err
	}

	// Set the check ID
	configDigest := config.FastDigest()

	if err := c.Configure(senderManager, configDigest, instance, config.InitConfig, config.Source); err != nil {
		return c, err
	}

	return c, nil
}
