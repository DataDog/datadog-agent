// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loaders

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type LoaderOne struct{}

func (lo LoaderOne) Name() string {
	return "loader_one"
}

//nolint:revive // TODO(AML) Fix revive linter
func (lo LoaderOne) Load(_ sender.SenderManager, _ integration.Config, _ integration.Data) (check.Check, error) {
	var c check.Check
	return c, nil
}

type LoaderTwo struct{}

func (lt LoaderTwo) Name() string {
	return "loader_two"
}

//nolint:revive // TODO(AML) Fix revive linter
func (lt LoaderTwo) Load(_ sender.SenderManager, _ integration.Config, _ integration.Data) (check.Check, error) {
	var c check.Check
	return c, nil
}

type LoaderThree struct{}

func (lt *LoaderThree) Name() string {
	return "loader_three"
}

//nolint:revive // TODO(AML) Fix revive linter
func (lt *LoaderThree) Load(_ sender.SenderManager, _ integration.Config, _ integration.Data) (check.Check, error) {
	var c check.Check
	return c, nil
}

func TestLoaderCatalog(t *testing.T) {
	l1 := LoaderOne{}
	factory1 := func(sender.SenderManager, optional.Option[integrations.Component], tagger.Component) (check.Loader, error) {
		return l1, nil
	}
	l2 := LoaderTwo{}
	factory2 := func(sender.SenderManager, optional.Option[integrations.Component], tagger.Component) (check.Loader, error) {
		return l2, nil
	}
	var l3 *LoaderThree
	factory3 := func(sender.SenderManager, optional.Option[integrations.Component], tagger.Component) (check.Loader, error) {
		return l3, errors.New("error")
	}

	RegisterLoader(20, factory1)
	RegisterLoader(10, factory2)
	RegisterLoader(30, factory3)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := optional.NewNoneOption[integrations.Component]()
	tagger := nooptagger.NewTaggerClient()
	require.Len(t, LoaderCatalog(senderManager, logReceiver, tagger), 2)
	assert.Equal(t, l1, LoaderCatalog(senderManager, logReceiver, tagger)[1])
	assert.Equal(t, l2, LoaderCatalog(senderManager, logReceiver, tagger)[0])
}
