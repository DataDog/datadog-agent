// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckFactory factory function type to instantiate checks
type CheckFactory func() check.Check

// Catalog keeps track of Go checks by name
var catalog = make(map[string]CheckFactory)

// RegisterCheck adds a check to the catalog
func RegisterCheck(name string, c CheckFactory) {
	catalog[name] = c
}

// GetRegisteredFactoryKeys get the keys for all registered factories
func GetRegisteredFactoryKeys() []string {
	factoryKeys := []string{}
	for name := range catalog {
		factoryKeys = append(factoryKeys, name)
	}

	return factoryKeys
}

// GoCheckLoader is a specific loader for checks living in this package
type GoCheckLoader struct{}

// NewGoCheckLoader creates a loader for go checks
func NewGoCheckLoader() (*GoCheckLoader, error) {
	return &GoCheckLoader{}, nil
}

// Name return returns Go loader name
func (gl *GoCheckLoader) Name() string {
	return "core"
}

// Load returns a Go check
func (gl *GoCheckLoader) Load(config integration.Config, instance integration.Data) (check.Check, error) {
	var c check.Check

	factory, found := catalog[config.Name]
	if !found {
		msg := fmt.Sprintf("Check %s not found in Catalog", config.Name)
		return c, fmt.Errorf(msg)
	}

	c = factory()
	if err := c.Configure(config.FastDigest(), instance, config.InitConfig, config.Source); err != nil {
		log.Errorf("core.loader: could not configure check %s: %s", c, err)
		msg := fmt.Sprintf("Could not configure check %s: %s", c, err)
		return c, fmt.Errorf(msg)
	}

	return c, nil
}

func (gl *GoCheckLoader) String() string {
	return "Core Check Loader"
}

func init() {
	factory := func() (check.Loader, error) {
		return NewGoCheckLoader()
	}

	loaders.RegisterLoader(30, factory)
}
