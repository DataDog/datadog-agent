// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadKprobeProfile(t *testing.T) {
	m, err := readKprobeProfile("./testdata/kprobe_profile")
	require.NoError(t, err)

	expected := map[string]KprobeStats{
		"ptcp_v4_connect":      {Hits: 497467, Misses: 0},
		"r_tcp_v4_connect_0":   {Hits: 0, Misses: 0},
		"rtcp_v4_connect":      {Hits: 497127, Misses: 28},
		"ptcp_sendmsg":         {Hits: 56868656, Misses: 0},
		"ptcp_cleanup_rbuf":    {Hits: 121249908, Misses: 0},
		"pudp_sendmsg":         {Hits: 19215832, Misses: 0},
		"r_udp_recvmsg_0":      {Hits: 0, Misses: 0},
		"rudp_recvmsg":         {Hits: 30238969, Misses: 0},
		"ptcp_retransmit_skb":  {Hits: 26450, Misses: 0},
		"ptcp_v6_connect":      {Hits: 99, Misses: 0},
		"r_tcp_v6_connect_0":   {Hits: 0, Misses: 0},
		"rtcp_v6_connect":      {Hits: 99, Misses: 0},
		"ptcp_close":           {Hits: 938320, Misses: 0},
		"pudp_recvmsg":         {Hits: 30212146, Misses: 0},
		"r_inet_csk_accept_0":  {Hits: 0, Misses: 0},
		"rinet_csk_accept":     {Hits: 1047728, Misses: 0},
		"ptcp_v4_destroy_sock": {Hits: 938590, Misses: 0},
	}

	assert.Equal(t, expected, m)
}
