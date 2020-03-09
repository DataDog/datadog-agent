package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadKprobeProfile(t *testing.T) {
	m, err := readKprobeProfile("./testdata/kprobe_profile")
	require.NoError(t, err)

	expected := map[string]kprobeStats{
		"ptcp_v4_connect":      {hits: 497467, miss: 0},
		"r_tcp_v4_connect_0":   {hits: 0, miss: 0},
		"rtcp_v4_connect":      {hits: 497127, miss: 28},
		"ptcp_sendmsg":         {hits: 56868656, miss: 0},
		"ptcp_cleanup_rbuf":    {hits: 121249908, miss: 0},
		"pudp_sendmsg":         {hits: 19215832, miss: 0},
		"r_udp_recvmsg_0":      {hits: 0, miss: 0},
		"rudp_recvmsg":         {hits: 30238969, miss: 0},
		"ptcp_retransmit_skb":  {hits: 26450, miss: 0},
		"ptcp_v6_connect":      {hits: 99, miss: 0},
		"r_tcp_v6_connect_0":   {hits: 0, miss: 0},
		"rtcp_v6_connect":      {hits: 99, miss: 0},
		"ptcp_close":           {hits: 938320, miss: 0},
		"pudp_recvmsg":         {hits: 30212146, miss: 0},
		"r_inet_csk_accept_0":  {hits: 0, miss: 0},
		"rinet_csk_accept":     {hits: 1047728, miss: 0},
		"ptcp_v4_destroy_sock": {hits: 938590, miss: 0},
	}

	assert.Equal(t, expected, m)
}
