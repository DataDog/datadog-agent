// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckFactory factory function type to instantiate checks
type CheckFactory func() check.Check

// Catalog keeps track of Go checks by name
var catalog = make(map[string]CheckFactory)

// GoCheckLoaderName is the name of the Go loader
const GoCheckLoaderName string = "core"

// RegisterCheck adds a check to the catalog
func RegisterCheck(name string, checkFactory option.Option[func() check.Check]) {
	if v, ok := checkFactory.Get(); ok {
		catalog[name] = v
	}
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
func (*GoCheckLoader) Name() string {
	return GoCheckLoaderName
}

// Load returns a Go check
func (gl *GoCheckLoader) Load(senderManger sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	var c check.Check

	factory, found := catalog[config.Name]
	if !found {
		msg := fmt.Sprintf("Check %s not found in Catalog", config.Name)
		return c, errors.New(msg)
	}

	c = factory()
	if err := c.Configure(senderManger, config.FastDigest(), instance, config.InitConfig, config.Source); err != nil {
		if errors.Is(err, check.ErrSkipCheckInstance) {
			return c, err
		}
		log.Errorf("core.loader: could not configure check %s: %s", c, err)
		msg := fmt.Sprintf("Could not configure check %s: %s", c, err)
		return c, errors.New(msg)
	}

	return c, nil
}

func (gl *GoCheckLoader) String() string {
	return "Core Check Loader"
}

func init() {
	factory := func(sender.SenderManager, option.Option[integrations.Component], tagger.Component) (check.Loader, int, error) {
		loader, err := NewGoCheckLoader()
		return loader, 30, err
	}

	loaders.RegisterLoader(factory)
}
