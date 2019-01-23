// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package corechecks

import (
	"fmt"
	"strings"

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

// GetCheckFactory grabs factory for specific check
func GetCheckFactory(name string) CheckFactory {
	f, ok := catalog[name]
	if !ok {
		return nil
	}
	return f
}

// GoCheckLoader is a specific loader for checks living in this package
type GoCheckLoader struct{}

// NewGoCheckLoader creates a loader for go checks
func NewGoCheckLoader() (*GoCheckLoader, error) {
	return &GoCheckLoader{}, nil
}

// Load returns a list of checks, one for every configuration instance found in `config`
func (gl *GoCheckLoader) Load(config integration.Config) ([]check.Check, error) {
	checks := []check.Check{}

	// If JMX check, just skip - coincidence
	if check.IsJMXConfig(config.Name, config.InitConfig) {
		return checks, fmt.Errorf("check %s appears to be a JMX check - skipping", config.Name)
	}

	factory, found := catalog[config.Name]
	if !found {
		msg := fmt.Sprintf("Check %s not found in Catalog", config.Name)
		return checks, fmt.Errorf(msg)
	}

	errors := []string{}
	for _, instance := range config.Instances {
		newCheck := factory()
		if err := newCheck.Configure(instance, config.InitConfig); err != nil {
			errors = append(errors, fmt.Sprintf("Could not configure check %s: %s", newCheck, err))
			log.Errorf("core.loader: could not configure check %s: %s", newCheck, err)
			continue
		}
		checks = append(checks, newCheck)
	}

	if len(errors) != 0 {
		return checks, fmt.Errorf(strings.Join(errors, "\n"))
	}

	return checks, nil
}

func (gl *GoCheckLoader) String() string {
	return "Core Check Loader"
}

func init() {
	factory := func() (check.Loader, error) {
		return NewGoCheckLoader()
	}

	loaders.RegisterLoader(20, factory)
}
