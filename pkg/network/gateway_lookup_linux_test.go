// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || linux_bpf

package network

import (
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestIsVPCRouterIP(t *testing.T) {
	tests := []struct {
		name    string
		gateway util.Address
		cidr    string
		want    bool
	}{
		{
			name:    "gateway is VPC router for /24 subnet",
			gateway: util.AddressFromNetIP(net.ParseIP("10.0.1.1")),
			cidr:    "10.0.1.0/24",
			want:    true,
		},
		{
			name:    "gateway is not VPC router",
			gateway: util.AddressFromNetIP(net.ParseIP("10.0.1.55")),
			cidr:    "10.0.1.0/24",
			want:    false,
		},
		{
			name:    "gateway is VPC router for /16 subnet",
			gateway: util.AddressFromNetIP(net.ParseIP("10.0.0.1")),
			cidr:    "10.0.0.0/16",
			want:    true,
		},
		{
			name:    "gateway is VPC router for 172.16 subnet",
			gateway: util.AddressFromNetIP(net.ParseIP("172.16.0.1")),
			cidr:    "172.16.0.0/20",
			want:    true,
		},
		{
			name:    "empty CIDR returns false",
			gateway: util.AddressFromNetIP(net.ParseIP("10.0.0.1")),
			cidr:    "",
			want:    false,
		},
		{
			name:    "malformed CIDR returns false",
			gateway: util.AddressFromNetIP(net.ParseIP("10.0.0.1")),
			cidr:    "not-a-cidr",
			want:    false,
		},
		{
			name:    "gateway in different subnet",
			gateway: util.AddressFromNetIP(net.ParseIP("10.0.2.1")),
			cidr:    "10.0.1.0/24",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isVPCRouterIP(tt.gateway, tt.cidr))
		})
	}
}

func TestLookupWithIPs_SkipsVPCRouter(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRouteCache := NewMockRouteCache(ctrl)
	subnetCache, _ := simplelru.NewLRU[int, any](10, nil)

	gl := &gatewayLookup{
		routeCache:  mockRouteCache,
		subnetCache: subnetCache,
	}

	src := util.AddressFromString("10.0.1.5")
	dst := util.AddressFromString("10.0.1.100")
	ifIndex := 3

	// Pre-populate subnet cache with a Via that includes the CIDR
	subnetCache.Add(ifIndex, &Via{
		Subnet: Subnet{Alias: "subnet-abc", Cidr: "10.0.1.0/24"},
	})

	// Route has gateway = VPC router IP for subnet 10.0.1.0/24
	mockRouteCache.EXPECT().Get(src, dst, uint32(0)).Return(Route{
		Gateway: util.AddressFromNetIP(net.ParseIP("10.0.1.1")),
		IfIndex: ifIndex,
	}, true)

	// Should return nil because gateway is the VPC router
	assert.Nil(t, gl.LookupWithIPs(src, dst, 0))

	// Now test that a non-router gateway DOES return the Via
	mockRouteCache.EXPECT().Get(src, dst, uint32(0)).Return(Route{
		Gateway: util.AddressFromNetIP(net.ParseIP("10.0.1.55")),
		IfIndex: ifIndex,
	}, true)

	via := gl.LookupWithIPs(src, dst, 0)
	assert.NotNil(t, via)
	assert.Equal(t, "subnet-abc", via.Subnet.Alias)
}
