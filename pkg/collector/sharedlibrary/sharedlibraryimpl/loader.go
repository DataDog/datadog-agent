// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package sharedlibrarycheck

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckLoaderName is the name of the Shared Library loader
const CheckLoaderName string = "sharedlibrary"

// CheckLoader is a specific loader for checks living in this package
type CheckLoader struct {
	loader ffi.LibraryLoader
}

func newCheckLoader(_ sender.SenderManager, _ option.Option[integrations.Component], _ tagger.Component, _ workloadfilter.Component, loader ffi.LibraryLoader) (*CheckLoader, error) {
	return &CheckLoader{
		loader: loader,
	}, nil
}

// Name returns Shared Library loader name
func (*CheckLoader) Name() string {
	return CheckLoaderName
}

func (*CheckLoader) String() string {
	return "Shared Library Loader"
}

// Load returns a Shared Library check
func (sl *CheckLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, _ int) (check.Check, error) {
	// we need to dynamically compute the shared libraries path because their extensions are platform dependent
	// we could also have collisions with existing shared libraries
	libPath := sl.loader.ComputeLibraryPath(config.Name)

	// open the library and get pointers to its symbols through the library loader
	lib, err := sl.loader.Open(libPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to load shared library at %s: %w", libPath, err)
	}

	// Create the check
	c, err := newCheck(senderManager, config.Name, sl.loader, lib)
	if err != nil {
		return c, err
	}

	configDigest := config.FastDigest()

	// pass the configuration to the check
	if err := c.Configure(senderManager, configDigest, instance, config.InitConfig, config.Source); err != nil {
		return c, err
	}

	// check version -- fallback to "unversioned" version if the version cannot be retrieved from the library
	version, err := sl.loader.Version(lib)
	if err != nil {
		c.version = "unversioned"
	} else {
		c.version = version
	}

	return c, nil
}
