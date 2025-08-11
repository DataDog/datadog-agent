// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestExternalHostTags(t *testing.T) {
	host1 := "localhost"
	host2 := "127.0.0.1"
	sourceType := "vsphere"
	tags1 := []string{"foo", "bar"}
	tags2 := []string{"baz"}
	eTags1 := externalhost.ExternalTags{sourceType: tags1}
	eTags2 := externalhost.ExternalTags{sourceType: tags2}
	externalhost.SetExternalTags(host1, sourceType, tags1)
	externalhost.SetExternalTags(host2, sourceType, tags2)

	c := newCollector(fxutil.Test[dependencies](t,
		core.MockBundle(),
		demultiplexerimpl.MockModule(),
		haagentmock.Module(),
		fx.Provide(func() option.Option[agenttelemetry.Component] {
			return option.None[agenttelemetry.Component]()
		}),
		fx.Provide(func() option.Option[serializer.MetricSerializer] {
			return option.None[serializer.MetricSerializer]()
		}),
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{"check_cancel_timeout": 500 * time.Millisecond},
		})))

	pl := c.GetPayload(context.Background())
	hpl := pl.ExternalhostTags
	assert.Len(t, hpl, 2)
	for _, elem := range hpl {
		if elem[0] == host1 {
			assert.Equal(t, eTags1, elem[1])
		} else if elem[0] == host2 {
			assert.Equal(t, eTags2, elem[1])
		} else {
			assert.Fail(t, "Unexpected value for hostname: %s", elem[0])
		}
	}
}
