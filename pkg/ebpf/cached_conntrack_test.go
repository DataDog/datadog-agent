// +build linux_bpf

package ebpf

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestEnsureConntrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errorCreator := func(_ int) (netlink.Conntrack, error) { return nil, assert.AnError }

	cache := newCachedConntrack("/proc", errorCreator, 1)
	defer cache.Close()

	ctrk, err := cache.ensureConntrack(0, os.Getpid())
	require.Nil(t, ctrk)
	require.Error(t, err)
	require.Equal(t, assert.AnError, err)

	m := netlink.NewMockConntrack(ctrl)
	n := 0
	creator := func(_ int) (netlink.Conntrack, error) {
		n++
		return m, nil
	}

	cache = newCachedConntrack("/proc", creator, 1)
	defer cache.Close()

	// once when cache.Close() is called, another when eviction happens
	m.EXPECT().Close().Times(2)

	ctrk, err = cache.ensureConntrack(1234, os.Getpid())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	ctrk.Close()

	// call again, should get the cached Conntrack
	ctrk, err = cache.ensureConntrack(1234, os.Getpid())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	ctrk.Close()

	// evict the lone conntrack in the cache
	ctrk, err = cache.ensureConntrack(1235, os.Getpid())
	require.NoError(t, err)
	require.Equal(t, 2, n)
	ctrk.Close()
}

func TestCachedConntrackExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := netlink.NewMockConntrack(ctrl)
	n := 0
	creator := func(_ int) (netlink.Conntrack, error) {
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
	ct := newConnTuple(os.Getpid(), 1234, saddr, daddr, sport, dport, network.TCP)

	m.EXPECT().Exists(gomock.Not(gomock.Nil())).Times(1).DoAndReturn(func(c *netlink.Con) (bool, error) {
		require.Equal(t, saddr.String(), c.Origin.Src.String())
		require.Equal(t, daddr.String(), c.Origin.Dst.String())
		require.Equal(t, sport, *c.Origin.Proto.SrcPort)
		require.Equal(t, dport, *c.Origin.Proto.DstPort)
		require.Equal(t, uint8(unix.IPPROTO_TCP), *c.Origin.Proto.Number)
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
			require.Nil(t, c.Reply)
			require.Equal(t, saddr.String(), c.Origin.Src.String())
			require.Equal(t, daddr.String(), c.Origin.Dst.String())
			require.Equal(t, sport, *c.Origin.Proto.SrcPort)
			require.Equal(t, dport, *c.Origin.Proto.DstPort)
			require.Equal(t, uint8(unix.IPPROTO_TCP), *c.Origin.Proto.Number)
			return false, nil
		}

		if i == 2 {
			require.Nil(t, c.Origin)
			require.Equal(t, saddr.String(), c.Reply.Src.String())
			require.Equal(t, daddr.String(), c.Reply.Dst.String())
			require.Equal(t, sport, *c.Reply.Proto.SrcPort)
			require.Equal(t, dport, *c.Reply.Proto.DstPort)
			require.Equal(t, uint8(unix.IPPROTO_TCP), *c.Reply.Proto.Number)
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
	creator := func(_ int) (netlink.Conntrack, error) {
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
	defer func() {
		for _, c := range ctrks {
			c.Close()
		}
	}()

	m.EXPECT().Close().Times(len(ctrks))
}
