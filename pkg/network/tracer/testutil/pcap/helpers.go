package pcap

import (
	"testing"

	"github.com/google/gopacket"
	gopcap "github.com/google/gopacket/pcap"
	"github.com/stretchr/testify/require"
)

func GetPacketSourceFromPCAP(t *testing.T, path string) *gopacket.PacketSource {
	t.Helper()

	handle, err := gopcap.OpenOffline(path)
	require.NoError(t, err)
	t.Cleanup(handle.Close)

	return gopacket.NewPacketSource(handle, handle.LinkType())
}
