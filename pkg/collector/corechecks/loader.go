// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// CheckFactory factory function type to instantiate checks
type CheckFactory func() check.Check

// Catalog keeps track of Go checks by name
var catalog = make(map[string]CheckFactory)

// RegisterCheck adds a check to the catalog
func RegisterCheck(name string, checkFactory optional.Option[func() check.Check]) {
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
func (gl *GoCheckLoader) Name() string {
	return "core"
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
	factory := func(sender.SenderManager, optional.Option[integrations.Component], tagger.Component) (check.Loader, error) {
		return NewGoCheckLoader()
	}

	loaders.RegisterLoader(30, factory)
}
