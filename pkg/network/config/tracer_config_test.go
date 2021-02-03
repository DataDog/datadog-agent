package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcSysGetInt(t *testing.T) {
	v := procSysGetInt("/proc", "foo", 123)
	require.Equal(t, v, 123)

	v = procSysGetInt("/proc", "net/netfilter/nf_conntrack_events", 12)
	require.NotEqual(t, 0, v)
	require.NotEqual(t, 12, v)
}

func TestSysUDPTimeout(t *testing.T) {
	v := procSysGetInt("/proc", "net/netfilter/nf_conntrack_udp_timeout", -1)
	require.NotEqual(t, v, -1)

	require.Equal(t, time.Duration(v)*time.Second, sysUDPTimeout())
}

func TestSysUDPStreamTimeout(t *testing.T) {
	v := procSysGetInt("/proc", "net/netfilter/nf_conntrack_udp_timeout_stream", -1)
	require.NotEqual(t, v, -1)

	require.Equal(t, time.Duration(v)*time.Second, sysUDPStreamTimeout())
}
