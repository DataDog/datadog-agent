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
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckFactory factory function type to instantiate checks
type CheckFactory func() check.Check

// LoadMode describes why a core check is being constructed.
type LoadMode string

const (
	// NormalLoadMode constructs a normal collector check.
	NormalLoadMode LoadMode = "normal"
	// ShadowLoadMode constructs a shadow collector check.
	ShadowLoadMode LoadMode = "shadow"
)

// ConstructionContext is passed to factories that need mode-specific
// dependencies while constructing a check instance.
type ConstructionContext struct {
	Mode LoadMode
}

// ContextualCheckFactory instantiates checks with construction context.
type ContextualCheckFactory func(ConstructionContext) check.Check

// Catalog keeps track of Go checks by name
var catalog = make(map[string]ContextualCheckFactory)

// GoCheckLoaderName is the name of the Go loader
const GoCheckLoaderName string = "core"

// RegisterCheck adds a check to the catalog
func RegisterCheck(name string, checkFactory option.Option[func() check.Check]) {
	if v, ok := checkFactory.Get(); ok {
		catalog[name] = func(ConstructionContext) check.Check {
			return v()
		}
	}
}

// RegisterContextualCheck adds a context-aware check factory to the catalog.
func RegisterContextualCheck(name string, checkFactory option.Option[func(ConstructionContext) check.Check]) {
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
type GoCheckLoader struct {
	mode LoadMode
}

// GoCheckLoaderOption configures a GoCheckLoader.
type GoCheckLoaderOption func(*GoCheckLoader)

// WithLoadMode configures the construction mode for loaded checks.
func WithLoadMode(mode LoadMode) GoCheckLoaderOption {
	return func(gl *GoCheckLoader) {
		gl.mode = mode
	}
}

// LoadMode returns the loader's check construction mode.
func (gl *GoCheckLoader) LoadMode() LoadMode {
	return gl.mode
}

// NewGoCheckLoader creates a loader for go checks
func NewGoCheckLoader(opts ...GoCheckLoaderOption) (*GoCheckLoader, error) {
	loader := &GoCheckLoader{mode: NormalLoadMode}
	for _, opt := range opts {
		opt(loader)
	}
	return loader, nil
}

// Name return returns Go loader name
func (*GoCheckLoader) Name() string {
	return GoCheckLoaderName
}

// Load returns a Go check
func (gl *GoCheckLoader) Load(senderManger sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (check.Check, error) {
	var c check.Check

	factory, found := catalog[config.Name]
	if !found {
		msg := fmt.Sprintf("Check %s not found in Catalog", config.Name)
		return c, errors.New(msg)
	}

	c = factory(ConstructionContext{Mode: gl.mode})
	if c == nil {
		msg := fmt.Sprintf("Check %s factory returned nil", config.Name)
		return c, errors.New(msg)
	}

	configSource := config.Source
	if instanceIndex >= 0 {
		configSource = fmt.Sprintf("%s[%d]", configSource, instanceIndex)
	}
	if err := c.Configure(senderManger, config.FastDigest(), instance, config.InitConfig, configSource, config.Provider); err != nil {
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
	factory := func(sender.SenderManager, option.Option[integrations.Component], tagger.Component, workloadfilter.Component) (check.Loader, int, error) {
		loader, err := NewGoCheckLoader()
		priority := 30
		if pkgconfigsetup.Datadog().GetBool("prioritize_go_check_loader") {
			priority = 10
		}
		return loader, priority, err
	}

	loaders.RegisterLoader(factory)
}
