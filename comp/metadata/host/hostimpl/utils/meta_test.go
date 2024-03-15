// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
)

func TestGetMeta(t *testing.T) {
	ctx := context.Background()
	cfg := fxutil.Test[config.Component](t, config.MockModule())

	meta := GetMeta(ctx, cfg)
	assert.NotEmpty(t, meta.SocketHostname)
	assert.NotEmpty(t, meta.Timezones)
	assert.NotEmpty(t, meta.SocketFqdn)
}

func TestGetMetaFromCache(t *testing.T) {
	ctx := context.Background()
	cfg := fxutil.Test[config.Component](t, config.MockModule())

	cache.Cache.Set(metaCacheKey, &Meta{
		SocketHostname: "socket_test",
		Timezones:      []string{"tz_test"},
	}, cache.NoExpiration)

	m := GetMetaFromCache(ctx, cfg)
	assert.Equal(t, "socket_test", m.SocketHostname)
	assert.Equal(t, []string{"tz_test"}, m.Timezones)
}
