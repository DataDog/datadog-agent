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

	expected := map[string]KProbeStats{
		"ptcp_v4_connect":      {Hits: 497467, Miss: 0},
		"r_tcp_v4_connect_0":   {Hits: 0, Miss: 0},
		"rtcp_v4_connect":      {Hits: 497127, Miss: 28},
		"ptcp_sendmsg":         {Hits: 56868656, Miss: 0},
		"ptcp_cleanup_rbuf":    {Hits: 121249908, Miss: 0},
		"pudp_sendmsg":         {Hits: 19215832, Miss: 0},
		"r_udp_recvmsg_0":      {Hits: 0, Miss: 0},
		"rudp_recvmsg":         {Hits: 30238969, Miss: 0},
		"ptcp_retransmit_skb":  {Hits: 26450, Miss: 0},
		"ptcp_v6_connect":      {Hits: 99, Miss: 0},
		"r_tcp_v6_connect_0":   {Hits: 0, Miss: 0},
		"rtcp_v6_connect":      {Hits: 99, Miss: 0},
		"ptcp_close":           {Hits: 938320, Miss: 0},
		"pudp_recvmsg":         {Hits: 30212146, Miss: 0},
		"r_inet_csk_accept_0":  {Hits: 0, Miss: 0},
		"rinet_csk_accept":     {Hits: 1047728, Miss: 0},
		"ptcp_v4_destroy_sock": {Hits: 938590, Miss: 0},
	}

	assert.Equal(t, expected, m)
}
