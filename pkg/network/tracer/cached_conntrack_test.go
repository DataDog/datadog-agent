// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestEnsureConntrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errorCreator := func(_ netns.NsHandle) (netlink.Conntrack, error) { return nil, assert.AnError }

	cache := newCachedConntrack("/proc", errorCreator, 1)
	defer cache.Close()

	ctrk, err := cache.ensureConntrack(0, os.Getpid())
	require.Nil(t, ctrk)
	require.Error(t, err)
	require.Equal(t, assert.AnError, err)

	m := netlink.NewMockConntrack(ctrl)
	n := 0
	creator := func(_ netns.NsHandle) (netlink.Conntrack, error) {
		n++
		return m, nil
	}

	cache = newCachedConntrack("/proc", creator, 1)
	defer cache.Close()

	// one for when eviction happens for the first conntrack instance
	// and the second one for when the cache is closed, and the second
	// remaining conntrack instance is closed
	m.EXPECT().Close().Times(2)

	_, err = cache.ensureConntrack(1234, os.Getpid())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// call again, should get the cached Conntrack
	_, err = cache.ensureConntrack(1234, os.Getpid())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// evict the lone conntrack in the cache
	_, err = cache.ensureConntrack(1235, os.Getpid())
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

func TestCachedConntrackIgnoreErrExists(t *testing.T) {
	cache := newCachedConntrack("/proc", func(_ netns.NsHandle) (netlink.Conntrack, error) {
		require.FailNow(t, "unexpected call to conntrack creator")
		return nil, nil
	}, 1)
	defer cache.Close()

	ctrk, err := cache.ensureConntrack(0, 0)
	require.Nil(t, ctrk)
	require.NoError(t, err)
}

func TestCachedConntrackExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := netlink.NewMockConntrack(ctrl)
	n := 0
	creator := func(_ netns.NsHandle) (netlink.Conntrack, error) {
		n++
		return m, nil
	}

	cache := newCachedConntrack("/proc", creator, 10)
	defer cache.Close()

	m.EXPECT().Close().Times(1)

	saddr := util.AddressFromString("1.2.3.4")
	daddr := util.AddressFromString("2.3.4.5")
	var sport uint16 = 23
	var dport uint16 = 223
	ct := &network.ConnectionStats{
		Pid:    uint32(os.Getpid()),
		NetNS:  1234,
		Source: saddr,
		Dest:   daddr,
		SPort:  sport,
		DPort:  dport,
		Type:   network.TCP,
		Family: network.AFINET,
	}

	m.EXPECT().Exists(gomock.Not(gomock.Nil())).Times(1).DoAndReturn(func(c *netlink.Con) (bool, error) {
		require.Equal(t, saddr.String(), c.Origin.Src.Addr().String())
		require.Equal(t, daddr.String(), c.Origin.Dst.Addr().String())
		require.Equal(t, sport, c.Origin.Src.Port())
		require.Equal(t, dport, c.Origin.Dst.Port())
		require.Equal(t, uint8(unix.IPPROTO_TCP), c.Origin.Proto)
		return true, nil
	})

	exists, err := cache.Exists(ct)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 1, n)

	i := 0
	m.EXPECT().Exists(gomock.Not(gomock.Nil())).Times(2).DoAndReturn(func(c *netlink.Con) (bool, error) {
		i++

		if i == 1 {
			require.True(t, c.Reply.IsZero())
			require.Equal(t, saddr.String(), c.Origin.Src.Addr().String())
			require.Equal(t, daddr.String(), c.Origin.Dst.Addr().String())
			require.Equal(t, sport, c.Origin.Src.Port())
			require.Equal(t, dport, c.Origin.Dst.Port())
			require.Equal(t, uint8(unix.IPPROTO_TCP), c.Origin.Proto)
			return false, nil
		}

		if i == 2 {
			require.True(t, c.Origin.IsZero())
			require.Equal(t, saddr.String(), c.Reply.Src.Addr().String())
			require.Equal(t, daddr.String(), c.Reply.Dst.Addr().String())
			require.Equal(t, sport, c.Reply.Src.Port())
			require.Equal(t, dport, c.Reply.Dst.Port())
			require.Equal(t, uint8(unix.IPPROTO_TCP), c.Reply.Proto)
			return true, nil
		}

		require.Fail(t, "unexpected call to Conntrack.Exists")
		return false, nil
	})

	exists, err = cache.Exists(ct)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 1, n)
}

func TestCachedConntrackClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := netlink.NewMockConntrack(ctrl)
	n := 0
	creator := func(_ netns.NsHandle) (netlink.Conntrack, error) {
		n++
		return m, nil
	}

	cache := newCachedConntrack("/proc", creator, 10)
	defer cache.Close()

	var ctrks []netlink.Conntrack
	for i := 0; i < 10; i++ {
		ctrk, err := cache.ensureConntrack(uint64(1234+i), os.Getpid())
		require.NoError(t, err)
		require.NotNil(t, ctrk)
		ctrks = append(ctrks, ctrk)
	}

	m.EXPECT().Close().Times(len(ctrks))
}
