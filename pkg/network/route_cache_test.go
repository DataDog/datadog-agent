// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestRouteCacheGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRouter(ctrl)

	tests := []struct {
		source, dest string
		netns        uint32

		route Route
		ok    bool

		times int
	}{
		{source: "127.0.0.1", dest: "127.0.0.1", route: Route{IfIndex: 0}, ok: true, times: 1},
		{source: "10.0.2.2", dest: "8.8.8.8", route: Route{Gateway: util.AddressFromString("10.0.2.1"), IfIndex: 1}, ok: true, times: 1},
		{source: "1.2.3.4", dest: "5.6.7.8", route: Route{}, ok: false, times: 2}, // 2 calls expected here since this is not going to be cached
	}

	cache := NewRouteCache(10, m)
	defer cache.Close()

	m.EXPECT().Close()

	// run through to fill up cache
	for _, te := range tests {
		source := util.AddressFromString(te.source)
		dest := util.AddressFromString(te.dest)
		m.EXPECT().Route(gomock.Eq(source), gomock.Eq(dest), gomock.Eq(te.netns)).
			Return(te.route, te.ok).
			Times(te.times)

		r, ok := cache.Get(source, dest, te.netns)
		require.Equal(t, te.route, r)
		require.Equal(t, te.ok, ok)
	}

	for _, te := range tests {
		source := util.AddressFromString(te.source)
		dest := util.AddressFromString(te.dest)
		r, ok := cache.Get(source, dest, te.netns)
		require.Equal(t, te.route, r)
		require.Equal(t, te.ok, ok)
	}
}

func TestRouteCacheTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRouter(ctrl)

	route := Route{Gateway: util.AddressFromString("1.1.1.1"), IfIndex: 0}
	m.EXPECT().Route(gomock.Any(), gomock.Any(), gomock.Any()).Return(route, true).Times(2)

	cache := newRouteCache(10, m, time.Millisecond)
	defer cache.Close()

	m.EXPECT().Close()

	source := util.AddressFromString("1.1.1.1")
	dest := util.AddressFromString("1.2.3.4")
	r, ok := cache.Get(source, dest, 0)
	require.True(t, ok)
	require.Equal(t, route, r)

	time.Sleep(2 * time.Millisecond)

	r, ok = cache.Get(source, dest, 0)
	require.True(t, ok)
	require.Equal(t, route, r)
}
