// +build linux_bpf

package ebpf

import (
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDNSSnooping(t *testing.T) {
	m, err := readBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	// Load socket filter
	cfg := NewDefaultConfig()
	err = m.Load(SectionsFromConfig(cfg))
	require.NoError(t, err)

	filter := m.SocketFilter("socket/dns_filter")
	require.NotNil(t, filter)

	reverseDNS, err := NewSocketFilterSnooper(filter)
	require.NoError(t, err)
	defer reverseDNS.Close()

	// Connect to golang.org. This will result in a DNS lookup which will be captured by SocketFilterSnooper
	conn, err := net.DialTimeout("tcp", "golang.org:80", 1*time.Second)
	require.NoError(t, err)

	// Get destination IP to compare against snooped DNS
	destIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	conn.Close()
	require.NoError(t, err)
	destAddr := util.AddressFromString(destIP)

	time.Sleep(100 * time.Millisecond)

	// Verify that the IP from the connections above maps to the right name
	payload := []ConnectionStats{{Dest: destAddr}}
	names := reverseDNS.Resolve(payload)
	require.Len(t, names, 1)
	assert.Contains(t, names[0].Dest, "golang.org")
}
