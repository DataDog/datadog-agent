// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/agentmetadata"
	serverlessinitimpl "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/impl"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type metadataLifecycle struct{ hooks []compdef.Hook }

func (l *metadataLifecycle) Append(hook compdef.Hook) { l.hooks = append(l.hooks, hook) }

func TestAppServiceMetadataLateIdentity(t *testing.T) {
	t.Setenv("WEBSITE_SITE_NAME", "test-site")
	t.Setenv("WEBSITE_OWNER_NAME", "")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "")
	synctest.Test(t, func(t *testing.T) {
		s := serializermock.NewMetricSerializer(t)
		lc := &metadataLifecycle{}
		p := serverlessinitimpl.NewComponent(serverlessinitimpl.Requires{
			Lc: lc,
			Config: config.NewMockWithOverrides(t, map[string]any{
				"serverless.inventory_enabled":      true,
				"inventories_first_run_delay":       0,
				"inventories_configuration_enabled": false,
			}),
			Log:        logmock.New(t),
			Hostname:   fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule()),
			Provider:   &cloudservice.AppService{},
			Serializer: s,
		})
		require.NotNil(t, p.Provider.Callback, "missing initial identity must not disable collection")
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() { p.Provider.Callback(ctx); close(done) }()
		synctest.Wait()
		s.AssertNotCalled(t, "SendMetadata", mock.Anything)
		s.On("SendMetadata", mock.MatchedBy(func(payload *agentmetadata.Payload) bool {
			return payload.Metadata["resource_id"] == "/subscriptions/sub/resourcegroups/group/providers/microsoft.web/sites/test-site"
		})).Return(nil).Once()
		t.Setenv("WEBSITE_OWNER_NAME", "sub+webspace")
		t.Setenv("WEBSITE_RESOURCE_GROUP", "group")
		<-done
		require.Len(t, lc.hooks, 1)
		assert.NoError(t, lc.hooks[0].OnStop(t.Context()))
		s.AssertNumberOfCalls(t, "SendMetadata", 1)
	})
}
