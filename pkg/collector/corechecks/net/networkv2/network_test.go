// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

import (
	"bufio"
	"bytes"
	"slices"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/safchain/ethtool"
)

type fakeNetworkStats struct {
	counterStats                 []net.IOCountersStat
	counterStatsError            error
	protoCountersStats           []net.ProtoCountersStat
	protoCountersStatsError      error
	connectionStatsUDP4          []net.ConnectionStat
	connectionStatsUDP4Error     error
	connectionStatsUDP6          []net.ConnectionStat
	connectionStatsUDP6Error     error
	connectionStatsTCP4          []net.ConnectionStat
	connectionStatsTCP4Error     error
	connectionStatsTCP6          []net.ConnectionStat
	connectionStatsTCP6Error     error
	netstatAndSnmpCountersValues map[string]net.ProtoCountersStat
	netstatAndSnmpCountersError  error
	getProcPath                  string
}

// IOCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) IOCounters(_ bool) ([]net.IOCountersStat, error) {
	return n.counterStats, n.counterStatsError
}

// ProtoCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) ProtoCounters(_ []string) ([]net.ProtoCountersStat, error) {
	return n.protoCountersStats, n.protoCountersStatsError
}

// Connections returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) Connections(kind string) ([]net.ConnectionStat, error) {
	switch kind {
	case "udp4":
		return n.connectionStatsUDP4, n.connectionStatsUDP4Error
	case "udp6":
		return n.connectionStatsUDP6, n.connectionStatsUDP6Error
	case "tcp4":
		return n.connectionStatsTCP4, n.connectionStatsTCP4Error
	case "tcp6":
		return n.connectionStatsTCP6, n.connectionStatsTCP6Error
	}
	return nil, nil
}

func (n *fakeNetworkStats) NetstatAndSnmpCounters(_ []string) (map[string]net.ProtoCountersStat, error) {
	return n.netstatAndSnmpCountersValues, n.netstatAndSnmpCountersError
}

func (n *fakeNetworkStats) GetProcPath() string {
	return n.getProcPath
}

func (n *fakeNetworkStats) GetNetProcBasePath() string {
	return n.getProcPath
}

type MockEthtool struct {
	mock.Mock
}

func (f *MockEthtool) DriverInfo(iface string) (ethtool.DrvInfo, error) {
	if iface == "eth0" {
		return ethtool.DrvInfo{
			Driver:  "ena",
			Version: "mock_version",
		}, nil
	}
	if iface == "virtio_net_mock" {
		return ethtool.DrvInfo{
			Driver:  "virtio_net",
			Version: "mock_version",
		}, nil
	}
	if iface == "mlx5_core_mock" {
		return ethtool.DrvInfo{
			Driver:  "mlx5_core",
			Version: "mock_version",
		}, nil
	}
	if iface == "hv_netvsc_mock" {
		return ethtool.DrvInfo{
			Driver:  "hv_netvsc",
			Version: "mock_version",
		}, nil
	}
	if iface == "gve_mock" {
		return ethtool.DrvInfo{
			Driver:  "gve",
			Version: "mock_version",
		}, nil
	}
	if iface == "veth_mock" {
		return ethtool.DrvInfo{
			Driver:  "veth",
			Version: "mock_version",
		}, nil
	}

	if iface == "enodev_drvinfo_iface" {
		return ethtool.DrvInfo{}, unix.ENODEV
	}
	if iface == "enodev_stats_iface" {
		return ethtool.DrvInfo{
			Driver:  "ena",
			Version: "mock_version",
		}, nil
	}
	return ethtool.DrvInfo{}, unix.ENOTTY
}

func (f *MockEthtool) Stats(iface string) (map[string]uint64, error) {
	if iface == "eth0" {
		return map[string]uint64{
			"queue_0_tx_bytes": 12345,
			"rx_bytes[0]":      67890,
			"cpu0_rx_xdp_tx":   123,
			"tx_timeout":       456,
			"tx_queue_dropped": 789, // Tests queue name parsing
		}, nil
	}
	if iface == "gve_mock" {
		return map[string]uint64{
			"rx_packets":                       1,
			"tx_packets":                       2,
			"rx_bytes":                         3,
			"tx_bytes":                         4,
			"rx_dropped":                       5,
			"tx_dropped":                       6,
			"tx_timeouts":                      7,
			"rx_skb_alloc_fail":                8,
			"rx_buf_alloc_fail":                9,
			"rx_desc_err_dropped_pkt":          10,
			"interface_up_cnt":                 11,
			"interface_down_cnt":               12,
			"reset_cnt":                        13,
			"page_alloc_fail":                  14,
			"dma_mapping_error":                15,
			"stats_report_trigger_cnt":         16,
			"rx_posted_desc[0]":                17,
			"rx_completed_desc[0]":             18,
			"rx_consumed_desc[0]":              19,
			"rx_bytes[0]":                      20,
			"rx_cont_packet_cnt[0]":            21,
			"rx_frag_flip_cnt[0]":              22,
			"rx_frag_copy_cnt[0]":              23,
			"rx_frag_alloc_cnt[0]":             24,
			"rx_dropped_pkt[0]":                25,
			"rx_copybreak_pkt[0]":              26,
			"rx_copied_pkt[0]":                 27,
			"rx_queue_drop_cnt[0]":             28,
			"rx_no_buffers_posted[0]":          29,
			"rx_drops_packet_over_mru[0]":      30,
			"rx_drops_invalid_checksum[0]":     31,
			"rx_xdp_aborted[0]":                32,
			"rx_xdp_drop[0]":                   33,
			"rx_xdp_pass[0]":                   34,
			"rx_xdp_tx[0]":                     35,
			"rx_xdp_redirect[0]":               36,
			"rx_xdp_tx_errors[0]":              37,
			"rx_xdp_redirect_errors[0]":        38,
			"rx_xdp_alloc_fails[0]":            39,
			"rx_posted_desc[12]":               40,
			"rx_completed_desc[12]":            41,
			"rx_consumed_desc[12]":             42,
			"rx_bytes[12]":                     43,
			"rx_cont_packet_cnt[12]":           44,
			"rx_frag_flip_cnt[12]":             45,
			"rx_frag_copy_cnt[12]":             46,
			"rx_frag_alloc_cnt[12]":            47,
			"rx_dropped_pkt[12]":               48,
			"rx_copybreak_pkt[12]":             49,
			"rx_copied_pkt[12]":                50,
			"rx_queue_drop_cnt[12]":            51,
			"rx_no_buffers_posted[12]":         52,
			"rx_drops_packet_over_mru[12]":     53,
			"rx_drops_invalid_checksum[12]":    54,
			"rx_xdp_aborted[12]":               55,
			"rx_xdp_drop[12]":                  56,
			"rx_xdp_pass[12]":                  57,
			"rx_xdp_tx[12]":                    58,
			"rx_xdp_redirect[12]":              59,
			"rx_xdp_tx_errors[12]":             60,
			"rx_xdp_redirect_errors[12]":       61,
			"rx_xdp_alloc_fails[12]":           62,
			"tx_posted_desc[0]":                63,
			"tx_completed_desc[0]":             64,
			"tx_consumed_desc[0]":              65,
			"tx_bytes[0]":                      66,
			"tx_wake[0]":                       67,
			"tx_stop[0]":                       68,
			"tx_event_counter[0]":              69,
			"tx_dma_mapping_error[0]":          70,
			"tx_xsk_wakeup[0]":                 71,
			"tx_xsk_done[0]":                   72,
			"tx_xsk_sent[0]":                   73,
			"tx_xdp_xmit[0]":                   74,
			"tx_xdp_xmit_errors[0]":            75,
			"tx_posted_desc[12]":               76,
			"tx_completed_desc[12]":            77,
			"tx_consumed_desc[12]":             78,
			"tx_bytes[12]":                     79,
			"tx_wake[12]":                      80,
			"tx_stop[12]":                      81,
			"tx_event_counter[12]":             82,
			"tx_dma_mapping_error[12]":         83,
			"tx_xsk_wakeup[12]":                84,
			"tx_xsk_done[12]":                  85,
			"tx_xsk_sent[12]":                  86,
			"tx_xdp_xmit[12]":                  87,
			"tx_xdp_xmit_errors[12]":           88,
			"adminq_prod_cnt":                  89,
			"adminq_cmd_fail":                  90,
			"adminq_timeouts":                  91,
			"adminq_describe_device_cnt":       92,
			"adminq_cfg_device_resources_cnt":  93,
			"adminq_register_page_list_cnt":    94,
			"adminq_unregister_page_list_cnt":  95,
			"adminq_create_tx_queue_cnt":       96,
			"adminq_create_rx_queue_cnt":       97,
			"adminq_destroy_tx_queue_cnt":      98,
			"adminq_destroy_rx_queue_cnt":      99,
			"adminq_dcfg_device_resources_cnt": 100,
			"adminq_set_driver_parameter_cnt":  101,
			"adminq_report_stats_cnt":          102,
			"adminq_report_link_speed_cnt":     103,
		}, nil
	}

	if iface == "mlx5_core_mock" {
		return map[string]uint64{
			"rx_packets":                      1,
			"rx_bytes":                        2,
			"tx_packets":                      3,
			"tx_bytes":                        4,
			"tx_tso_packets":                  5,
			"tx_tso_bytes":                    6,
			"tx_tso_inner_packets":            7,
			"tx_tso_inner_bytes":              8,
			"tx_added_vlan_packets":           9,
			"tx_nop":                          10,
			"tx_mpwqe_blks":                   11,
			"tx_mpwqe_pkts":                   12,
			"tx_tls_encrypted_packets":        13,
			"tx_tls_encrypted_bytes":          14,
			"tx_tls_ooo":                      15,
			"tx_tls_dump_packets":             16,
			"tx_tls_dump_bytes":               17,
			"tx_tls_resync_bytes":             18,
			"tx_tls_skip_no_sync_data":        19,
			"tx_tls_drop_no_sync_data":        20,
			"tx_tls_drop_bypass_req":          21,
			"rx_lro_packets":                  22,
			"rx_lro_bytes":                    23,
			"rx_gro_packets":                  24,
			"rx_gro_bytes":                    25,
			"rx_gro_skbs":                     26,
			"rx_gro_match_packets":            27,
			"rx_gro_large_hds":                28,
			"rx_ecn_mark":                     29,
			"rx_removed_vlan_packets":         30,
			"rx_csum_unnecessary":             31,
			"rx_csum_none":                    32,
			"rx_csum_complete":                33,
			"rx_csum_complete_tail":           34,
			"rx_csum_complete_tail_slow":      35,
			"rx_csum_unnecessary_inner":       36,
			"rx_xdp_drop":                     37,
			"rx_xdp_redirect":                 38,
			"rx_xdp_tx_xmit":                  39,
			"rx_xdp_tx_mpwqe":                 40,
			"rx_xdp_tx_inlnw":                 41,
			"rx_xdp_tx_nops":                  42,
			"rx_xdp_tx_full":                  43,
			"rx_xdp_tx_err":                   44,
			"rx_xdp_tx_cqe":                   45,
			"tx_csum_none":                    46,
			"tx_csum_partial":                 47,
			"tx_csum_partial_inner":           48,
			"tx_queue_stopped":                49,
			"tx_queue_dropped":                50,
			"tx_xmit_more":                    51,
			"tx_recover":                      52,
			"tx_cqes":                         53,
			"tx_queue_wake":                   54,
			"tx_cqe_err":                      55,
			"tx_xdp_xmit":                     56,
			"tx_xdp_mpwqe":                    57,
			"tx_xdp_inlnw":                    58,
			"tx_xdp_nops":                     59,
			"tx_xdp_full":                     60,
			"tx_xdp_err":                      61,
			"tx_xdp_cqes":                     62,
			"rx_wqe_err":                      63,
			"rx_mpwqe_filler_cqes":            64,
			"rx_mpwqe_filler_strides":         65,
			"rx_oversize_pkts_sw_drop":        66,
			"rx_buff_alloc_err":               67,
			"rx_cqe_compress_blks":            68,
			"rx_cqe_compress_pkts":            69,
			"rx_congst_umr":                   70,
			"rx_arfs_add":                     71,
			"rx_arfs_request_in":              72,
			"rx_arfs_request_out":             73,
			"rx_arfs_expired":                 74,
			"rx_arfs_err":                     75,
			"rx_recover":                      76,
			"rx_pp_alloc_fast":                77,
			"rx_pp_alloc_slow":                78,
			"rx_pp_alloc_slow_high_order":     79,
			"rx_pp_alloc_empty":               80,
			"rx_pp_alloc_refill":              81,
			"rx_pp_alloc_waive":               82,
			"rx_pp_recycle_cached":            83,
			"rx_pp_recycle_cache_full":        84,
			"rx_pp_recycle_ring":              85,
			"rx_pp_recycle_ring_full":         86,
			"rx_pp_recycle_released_ref":      87,
			"rx_tls_decrypted_packets":        88,
			"rx_tls_decrypted_bytes":          89,
			"rx_tls_resync_req_pkt":           90,
			"rx_tls_resync_req_start":         91,
			"rx_tls_resync_req_end":           92,
			"rx_tls_resync_req_skip":          93,
			"rx_tls_resync_res_ok":            94,
			"rx_tls_resync_res_retry":         95,
			"rx_tls_resync_res_skip":          96,
			"rx_tls_err":                      97,
			"ch_events":                       98,
			"ch_poll":                         99,
			"ch_arm":                          100,
			"ch_aff_change":                   101,
			"ch_force_irq":                    102,
			"ch_eq_rearm":                     103,
			"rx_xsk_packets":                  104,
			"rx_xsk_bytes":                    105,
			"rx_xsk_csum_complete":            106,
			"rx_xsk_csum_unnecessary":         107,
			"rx_xsk_csum_unnecessary_inner":   108,
			"rx_xsk_csum_none":                109,
			"rx_xsk_ecn_mark":                 110,
			"rx_xsk_removed_vlan_packets":     111,
			"rx_xsk_xdp_drop":                 112,
			"rx_xsk_xdp_redirect":             113,
			"rx_xsk_wqe_err":                  114,
			"rx_xsk_mpwqe_filler_cqes":        115,
			"rx_xsk_mpwqe_filler_strides":     116,
			"rx_xsk_oversize_pkts_sw_drop":    117,
			"rx_xsk_buff_alloc_err":           118,
			"rx_xsk_cqe_compress_blks":        119,
			"rx_xsk_cqe_compress_pkts":        120,
			"rx_xsk_congst_umr":               121,
			"tx_xsk_xmit":                     122,
			"tx_xsk_mpwqe":                    123,
			"tx_xsk_inlnw":                    124,
			"tx_xsk_full":                     125,
			"tx_xsk_err":                      126,
			"tx_xsk_cqes":                     127,
			"rx_out_of_buffer":                128,
			"rx_if_down_packets":              129,
			"rx_steer_missed_packets":         130,
			"rx_oversize_pkts_buffer":         131,
			"rx_vport_unicast_packets":        132,
			"rx_vport_unicast_bytes":          133,
			"tx_vport_unicast_packets":        134,
			"tx_vport_unicast_bytes":          135,
			"rx_vport_multicast_packets":      136,
			"rx_vport_multicast_bytes":        137,
			"tx_vport_multicast_packets":      138,
			"tx_vport_multicast_bytes":        139,
			"rx_vport_broadcast_packets":      140,
			"rx_vport_broadcast_bytes":        141,
			"tx_vport_broadcast_packets":      142,
			"tx_vport_broadcast_bytes":        143,
			"rx_vport_rdma_unicast_packets":   144,
			"rx_vport_rdma_unicast_bytes":     145,
			"tx_vport_rdma_unicast_packets":   146,
			"tx_vport_rdma_unicast_bytes":     147,
			"rx_vport_rdma_multicast_packets": 148,
			"rx_vport_rdma_multicast_bytes":   149,
			"tx_vport_rdma_multicast_packets": 150,
			"tx_vport_rdma_multicast_bytes":   151,
			"tx_packets_phy":                  152,
			"rx_packets_phy":                  153,
			"rx_crc_errors_phy":               154,
			"tx_bytes_phy":                    155,
			"rx_bytes_phy":                    156,
			"tx_multicast_phy":                157,
			"tx_broadcast_phy":                158,
			"rx_multicast_phy":                159,
			"rx_broadcast_phy":                160,
			"rx_in_range_len_errors_phy":      161,
			"rx_out_of_range_len_phy":         162,
			"rx_oversize_pkts_phy":            163,
			"rx_symbol_err_phy":               164,
			"tx_mac_control_phy":              165,
			"rx_mac_control_phy":              166,
			"rx_unsupported_op_phy":           167,
			"rx_pause_ctrl_phy":               168,
			"tx_pause_ctrl_phy":               169,
			"rx_discards_phy":                 170,
			"tx_discards_phy":                 171,
			"tx_errors_phy":                   172,
			"rx_undersize_pkts_phy":           173,
			"rx_fragments_phy":                174,
			"rx_jabbers_phy":                  175,
			"rx_64_bytes_phy":                 176,
			"rx_65_to_127_bytes_phy":          177,
			"rx_128_to_255_bytes_phy":         178,
			"rx_256_to_511_bytes_phy":         179,
			"rx_512_to_1023_bytes_phy":        180,
			"rx_1024_to_1518_bytes_phy":       181,
			"rx_1519_to_2047_bytes_phy":       182,
			"rx_2048_to_4095_bytes_phy":       183,
			"rx_4096_to_8191_bytes_phy":       184,
			"rx_8192_to_10239_bytes_phy":      185,
			"link_down_events_phy":            186,
			"rx_prio0_bytes":                  187,
			"rx_prio0_packets":                188,
			"rx_prio0_discards":               189,
			"tx_prio0_bytes":                  190,
			"tx_prio0_packets":                191,
			"rx_prio1_bytes":                  192,
			"rx_prio1_packets":                193,
			"rx_prio1_discards":               194,
			"tx_prio1_bytes":                  195,
			"tx_prio1_packets":                196,
			"module_unplug":                   197,
			"module_bus_stuck":                198,
			"module_high_temp":                199,
			"module_bad_shorted":              200,
			"ch0_events":                      201,
			"ch0_poll":                        202,
			"ch0_arm":                         203,
			"ch0_aff_change":                  204,
			"ch0_force_irq":                   205,
			"ch0_eq_rearm":                    206,
			"ch1_events":                      207,
			"ch1_poll":                        208,
			"ch1_arm":                         209,
			"ch1_aff_change":                  210,
			"ch1_force_irq":                   211,
			"ch1_eq_rearm":                    212,
			"rx0_packets":                     213,
			"rx0_bytes":                       214,
			"rx0_csum_complete":               215,
			"rx0_csum_complete_tail":          216,
			"rx0_csum_complete_tail_slow":     217,
			"rx0_csum_unnecessary":            218,
			"rx0_csum_unnecessary_inner":      219,
			"rx0_csum_none":                   220,
			"rx0_xdp_drop":                    221,
			"rx0_xdp_redirect":                222,
			"rx0_lro_packets":                 223,
			"rx0_lro_bytes":                   224,
			"rx0_gro_packets":                 225,
			"rx0_gro_bytes":                   226,
			"rx0_gro_skbs":                    227,
			"rx0_gro_match_packets":           228,
			"rx0_gro_large_hds":               229,
			"rx0_ecn_mark":                    230,
			"rx0_removed_vlan_packets":        231,
			"rx0_wqe_err":                     232,
			"rx0_mpwqe_filler_cqes":           233,
			"rx0_mpwqe_filler_strides":        234,
			"rx0_oversize_pkts_sw_drop":       235,
			"rx0_buff_alloc_err":              236,
			"rx0_cqe_compress_blks":           237,
			"rx0_cqe_compress_pkts":           238,
			"rx0_congst_umr":                  239,
			"rx0_arfs_add":                    240,
			"rx0_arfs_request_in":             241,
			"rx0_arfs_request_out":            242,
			"rx0_arfs_expired":                243,
			"rx0_arfs_err":                    244,
			"rx0_recover":                     245,
			"rx0_pp_alloc_fast":               246,
			"rx0_pp_alloc_slow":               247,
			"rx0_pp_alloc_slow_high_order":    248,
			"rx0_pp_alloc_empty":              249,
			"rx0_pp_alloc_refill":             250,
			"rx0_pp_alloc_waive":              251,
			"rx0_pp_recycle_cached":           252,
			"rx0_pp_recycle_cache_full":       253,
			"rx0_pp_recycle_ring":             254,
			"rx0_pp_recycle_ring_full":        255,
			"rx0_pp_recycle_released_ref":     256,
			"rx0_tls_decrypted_packets":       257,
			"rx0_tls_decrypted_bytes":         258,
			"rx0_tls_resync_req_pkt":          259,
			"rx0_tls_resync_req_start":        260,
			"rx0_tls_resync_req_end":          261,
			"rx0_tls_resync_req_skip":         262,
			"rx0_tls_resync_res_ok":           263,
			"rx0_tls_resync_res_retry":        264,
			"rx0_tls_resync_res_skip":         265,
			"rx0_tls_err":                     266,
			"rx0_xdp_tx_xmit":                 267,
			"rx0_xdp_tx_mpwqe":                268,
			"rx0_xdp_tx_inlnw":                269,
			"rx0_xdp_tx_nops":                 270,
			"rx0_xdp_tx_full":                 271,
			"rx0_xdp_tx_err":                  272,
			"rx0_xdp_tx_cqes":                 273,
			"rx12_packets":                    274,
			"rx12_bytes":                      275,
			"rx12_csum_complete":              276,
			"rx12_csum_complete_tail":         277,
			"rx12_csum_complete_tail_slow":    278,
			"rx12_csum_unnecessary":           279,
			"rx12_csum_unnecessary_inner":     280,
			"rx12_csum_none":                  281,
			"rx12_xdp_drop":                   282,
			"rx12_xdp_redirect":               283,
			"rx12_lro_packets":                284,
			"rx12_lro_bytes":                  285,
			"rx12_gro_packets":                286,
			"rx12_gro_bytes":                  287,
			"rx12_gro_skbs":                   288,
			"rx12_gro_match_packets":          289,
			"rx12_gro_large_hds":              290,
			"rx12_ecn_mark":                   291,
			"rx12_removed_vlan_packets":       292,
			"rx12_wqe_err":                    293,
			"rx12_mpwqe_filler_cqes":          294,
			"rx12_mpwqe_filler_strides":       295,
			"rx12_oversize_pkts_sw_drop":      296,
			"rx12_buff_alloc_err":             297,
			"rx12_cqe_compress_blks":          298,
			"rx12_cqe_compress_pkts":          299,
			"rx12_congst_umr":                 300,
			"rx12_arfs_add":                   301,
			"rx12_arfs_request_in":            302,
			"rx12_arfs_request_out":           303,
			"rx12_arfs_expired":               304,
			"rx12_arfs_err":                   305,
			"rx12_recover":                    306,
			"rx12_pp_alloc_fast":              307,
			"rx12_pp_alloc_slow":              308,
			"rx12_pp_alloc_slow_high_order":   309,
			"rx12_pp_alloc_empty":             310,
			"rx12_pp_alloc_refill":            311,
			"rx12_pp_alloc_waive":             312,
			"rx12_pp_recycle_cached":          313,
			"rx12_pp_recycle_cache_full":      314,
			"rx12_pp_recycle_ring":            315,
			"rx12_pp_recycle_ring_full":       316,
			"rx12_pp_recycle_released_ref":    317,
			"rx12_tls_decrypted_packets":      318,
			"rx12_tls_decrypted_bytes":        319,
			"rx12_tls_resync_req_pkt":         320,
			"rx12_tls_resync_req_start":       321,
			"rx12_tls_resync_req_end":         322,
			"rx12_tls_resync_req_skip":        323,
			"rx12_tls_resync_res_ok":          324,
			"rx12_tls_resync_res_retry":       325,
			"rx12_tls_resync_res_skip":        326,
			"rx12_tls_err":                    327,
			"rx12_xdp_tx_xmit":                328,
			"rx12_xdp_tx_mpwqe":               329,
			"rx12_xdp_tx_inlnw":               330,
			"rx12_xdp_tx_nops":                331,
			"rx12_xdp_tx_full":                332,
			"rx12_xdp_tx_err":                 333,
			"rx12_xdp_tx_cqes":                334,
			"tx0_packets":                     335,
			"tx0_bytes":                       336,
			"tx0_tso_packets":                 337,
			"tx0_tso_bytes":                   338,
			"tx0_tso_inner_packets":           339,
			"tx0_tso_inner_bytes":             340,
			"tx0_csum_partial":                341,
			"tx0_csum_partial_inner":          342,
			"tx0_added_vlan_packets":          343,
			"tx0_nop":                         344,
			"tx0_mpwqe_blks":                  345,
			"tx0_mpwqe_pkts":                  346,
			"tx0_tls_encrypted_packets":       347,
			"tx0_tls_encrypted_bytes":         348,
			"tx0_tls_ooo":                     349,
			"tx0_tls_dump_packets":            350,
			"tx0_tls_dump_bytes":              351,
			"tx0_tls_resync_bytes":            352,
			"tx0_tls_skip_no_sync_data":       353,
			"tx0_tls_drop_no_sync_data":       354,
			"tx0_tls_drop_bypass_req":         355,
			"tx0_csum_none":                   356,
			"tx0_stopped":                     357,
			"tx0_dropped":                     358,
			"tx0_xmit_more":                   359,
			"tx0_recover":                     360,
			"tx0_cqes":                        361,
			"tx0_wake":                        362,
			"tx0_cqe_err":                     363,
			"tx12_packets":                    364,
			"tx12_bytes":                      365,
			"tx12_tso_packets":                366,
			"tx12_tso_bytes":                  367,
			"tx12_tso_inner_packets":          368,
			"tx12_tso_inner_bytes":            369,
			"tx12_csum_partial":               370,
			"tx12_csum_partial_inner":         371,
			"tx12_added_vlan_packets":         372,
			"tx12_nop":                        373,
			"tx12_mpwqe_blks":                 374,
			"tx12_mpwqe_pkts":                 375,
			"tx12_tls_encrypted_packets":      376,
			"tx12_tls_encrypted_bytes":        377,
			"tx12_tls_ooo":                    378,
			"tx12_tls_dump_packets":           379,
			"tx12_tls_dump_bytes":             380,
			"tx12_tls_resync_bytes":           381,
			"tx12_tls_skip_no_sync_data":      382,
			"tx12_tls_drop_no_sync_data":      383,
			"tx12_tls_drop_bypass_req":        384,
			"tx12_csum_none":                  385,
			"tx12_stopped":                    386,
			"tx12_dropped":                    387,
			"tx12_xmit_more":                  388,
			"tx12_recover":                    389,
			"tx12_cqes":                       390,
			"tx12_wake":                       391,
			"tx12_cqe_err":                    392,
			"tx0_xdp_xmit":                    393,
			"tx0_xdp_mpwqe":                   394,
			"tx0_xdp_inlnw":                   395,
			"tx0_xdp_nops":                    396,
			"tx0_xdp_full":                    397,
			"tx0_xdp_err":                     398,
			"tx0_xdp_cqes":                    399,
			"tx12_xdp_xmit":                   400,
			"tx12_xdp_mpwqe":                  401,
			"tx12_xdp_inlnw":                  402,
			"tx12_xdp_nops":                   403,
			"tx12_xdp_full":                   404,
			"tx12_xdp_err":                    405,
			"tx12_xdp_cqes":                   406,
		}, nil
	}

	if iface == "hv_netvsc_mock" {
		return map[string]uint64{
			"tx_scattered":             1,
			"tx_no_memory":             2,
			"tx_no_space":              3,
			"tx_too_big":               4,
			"tx_busy":                  5,
			"tx_send_full":             6,
			"rx_comp_busy":             7,
			"rx_no_memory":             8,
			"stop_queue":               9,
			"wake_queue":               10,
			"vlan_error":               11,
			"vf_rx_packets":            12,
			"vf_rx_bytes":              13,
			"vf_tx_packets":            14,
			"vf_tx_bytes":              15,
			"vf_tx_dropped":            16,
			"tx_queue_0_packets":       17,
			"tx_queue_0_bytes":         18,
			"tx_queue_0_xdp_xmit":      19,
			"rx_queue_0_packets":       20,
			"rx_queue_0_bytes":         21,
			"rx_queue_0_xdp_drop":      22,
			"rx_queue_0_xdp_redirect":  23,
			"rx_queue_0_xdp_tx":        24,
			"tx_queue_12_packets":      25,
			"tx_queue_12_bytes":        26,
			"tx_queue_12_xdp_xmit":     27,
			"rx_queue_12_packets":      28,
			"rx_queue_12_bytes":        29,
			"rx_queue_12_xdp_drop":     30,
			"rx_queue_12_xdp_redirect": 31,
			"rx_queue_12_xdp_tx":       32,
			"cpu0_rx_packets":          33,
			"cpu0_rx_bytes":            34,
			"cpu0_tx_packets":          35,
			"cpu0_tx_bytes":            36,
			"cpu0_vf_rx_packets":       37,
			"cpu0_vf_rx_bytes":         38,
			"cpu0_vf_tx_packets":       39,
			"cpu0_vf_tx_bytes":         40,
			"cpu1_rx_packets":          41,
			"cpu1_rx_bytes":            42,
			"cpu1_tx_packets":          43,
			"cpu1_tx_bytes":            44,
			"cpu1_vf_rx_packets":       45,
			"cpu1_vf_rx_bytes":         46,
			"cpu1_vf_tx_packets":       47,
			"cpu1_vf_tx_bytes":         48,
		}, nil
	}

	if iface == "virtio_net_mock" {
		return map[string]uint64{
			"rx_queue_0_packets":        1,
			"rx_queue_0_bytes":          2,
			"rx_queue_0_drops":          3,
			"rx_queue_0_xdp_packets":    4,
			"rx_queue_0_xdp_tx":         5,
			"rx_queue_0_xdp_redirects":  6,
			"rx_queue_0_xdp_drops":      7,
			"rx_queue_0_kicks":          8,
			"rx_queue_12_packets":       9,
			"rx_queue_12_bytes":         10,
			"rx_queue_12_drops":         11,
			"rx_queue_12_xdp_packets":   12,
			"rx_queue_12_xdp_tx":        13,
			"rx_queue_12_xdp_redirects": 14,
			"rx_queue_12_xdp_drops":     15,
			"rx_queue_12_kicks":         16,
			"tx_queue_0_packets":        17,
			"tx_queue_0_bytes":          18,
			"tx_queue_0_xdp_tx":         19,
			"tx_queue_0_xdp_tx_drops":   20,
			"tx_queue_0_kicks":          21,
			"tx_queue_0_tx_timeouts":    22,
			"tx_queue_12_packets":       23,
			"tx_queue_12_bytes":         24,
			"tx_queue_12_xdp_tx":        25,
			"tx_queue_12_xdp_tx_drops":  26,
			"tx_queue_12_kicks":         27,
			"tx_queue_12_tx_timeouts":   28,
		}, nil
	}

	if iface == "ena_mock" {
		return map[string]uint64{
			"tx_timeout":                   1,
			"suspend":                      2,
			"resume":                       3,
			"wd_expired":                   4,
			"interface_up":                 5,
			"interface_down":               6,
			"admin_q_pause":                7,
			"bw_in_allowance_exceeded":     8,
			"bw_out_allowance_exceeded":    9,
			"pps_allowance_exceeded":       10,
			"conntrack_allowance_exceeded": 11,
			"linklocal_allowance_exceeded": 12,
			"queue_0_tx_cnt":               13,
			"queue_0_tx_bytes":             14,
			"queue_0_tx_queue_stop":        15,
			"queue_0_tx_queue_wakeup":      16,
			"queue_0_tx_dma_mapping_err":   17,
			"queue_0_tx_linearize":         18,
			"queue_0_tx_linearize_failed":  19,
			"queue_0_tx_napi_comp":         20,
			"queue_0_tx_tx_poll":           21,
			"queue_0_tx_doorbells":         22,
			"queue_0_tx_prepare_ctx_err":   23,
			"queue_0_tx_bad_req_id":        24,
			"queue_0_tx_llq_buffer_copy":   25,
			"queue_0_tx_missed_tx":         26,
			"queue_0_tx_unmask_interrupt":  27,
			"queue_0_rx_cnt":               28,
			"queue_0_rx_bytes":             29,
			"queue_0_rx_rx_copybreak_pkt":  30,
			"queue_0_rx_csum_good":         31,
			"queue_0_rx_refil_partial":     32,
			"queue_0_rx_csum_bad":          33,
			"queue_0_rx_page_alloc_fail":   34,
			"queue_0_rx_skb_alloc_fail":    35,
			"queue_0_rx_dma_mapping_err":   36,
			"queue_0_rx_bad_desc_num":      37,
			"queue_0_rx_bad_req_id":        38,
			"queue_0_rx_empty_rx_ring":     39,
			"queue_0_rx_csum_unchecked":    40,
			"queue_0_rx_xdp_aborted":       41,
			"queue_0_rx_xdp_drop":          42,
			"queue_0_rx_xdp_pass":          43,
			"queue_0_rx_xdp_tx":            44,
			"queue_0_rx_xdp_invalid":       45,
			"queue_0_rx_xdp_redirect":      46,
			"queue_12_tx_cnt":              47,
			"queue_12_tx_bytes":            48,
			"queue_12_tx_queue_stop":       49,
			"queue_12_tx_queue_wakeup":     50,
			"queue_12_tx_dma_mapping_err":  51,
			"queue_12_tx_linearize":        52,
			"queue_12_tx_linearize_failed": 53,
			"queue_12_tx_napi_comp":        54,
			"queue_12_tx_tx_poll":          55,
			"queue_12_tx_doorbells":        56,
			"queue_12_tx_prepare_ctx_err":  57,
			"queue_12_tx_bad_req_id":       58,
			"queue_12_tx_llq_buffer_copy":  59,
			"queue_12_tx_missed_tx":        60,
			"queue_12_tx_unmask_interrupt": 61,
			"queue_12_rx_cnt":              62,
			"queue_12_rx_bytes":            63,
			"queue_12_rx_rx_copybreak_pkt": 64,
			"queue_12_rx_csum_good":        65,
			"queue_12_rx_refil_partial":    66,
			"queue_12_rx_csum_bad":         67,
			"queue_12_rx_page_alloc_fail":  68,
			"queue_12_rx_skb_alloc_fail":   69,
			"queue_12_rx_dma_mapping_err":  70,
			"queue_12_rx_bad_desc_num":     71,
			"queue_12_rx_bad_req_id":       72,
			"queue_12_rx_empty_rx_ring":    73,
			"queue_12_rx_csum_unchecked":   74,
			"queue_12_rx_xdp_aborted":      75,
			"queue_12_rx_xdp_drop":         76,
			"queue_12_rx_xdp_pass":         77,
			"queue_12_rx_xdp_tx":           78,
			"queue_12_rx_xdp_invalid":      79,
			"queue_12_rx_xdp_redirect":     80,
			"ena_admin_q_aborted_cmd":      81,
			"ena_admin_q_submitted_cmd":    82,
			"ena_admin_q_completed_cmd":    83,
			"ena_admin_q_out_of_space":     84,
		}, nil
	}

	if iface == "veth_mock" {
		return map[string]uint64{
			"peer_ifindex":               1,
			"rx_queue_0_xdp_packets":     2,
			"rx_queue_0_xdp_bytes":       3,
			"rx_queue_0_drops":           4,
			"rx_queue_0_xdp_redirect":    5,
			"rx_queue_0_xdp_drops":       6,
			"rx_queue_0_xdp_tx":          7,
			"rx_queue_0_xdp_tx_errors":   8,
			"tx_queue_0_xdp_xmit":        9,
			"tx_queue_0_xdp_xmit_errors": 10,
			"rx_pp_alloc_fast":           11,
			"rx_pp_alloc_slow":           12,
			"rx_pp_alloc_slow_ho":        13,
			"rx_pp_alloc_empty":          14,
			"rx_pp_alloc_refill":         15,
			"rx_pp_alloc_waive":          16,
			"rx_pp_recycle_cached":       17,
			"rx_pp_recycle_cache_full":   18,
			"rx_pp_recycle_ring":         19,
			"rx_pp_recycle_ring_full":    20,
			"rx_pp_recycle_released_ref": 21,
		}, nil
	}

	if iface == "enodev_stats_iface" {
		return nil, unix.ENODEV
	}
	return nil, unix.ENOTTY
}

func (f *MockEthtool) Close() {
}

func TestEthtoolParsing(t *testing.T) {
	testcases := []struct {
		name  string
		iface string
		want  map[string]map[string]uint64
	}{
		{
			name:  "skips unsupported NIC",
			iface: "veth_mock",
			want:  map[string]map[string]uint64{},
		},
		{
			name:  "parses virtio_net",
			iface: "virtio_net_mock",
			want: map[string]map[string]uint64{
				"queue:0": {
					"virtio_net.queue.rx_bytes":         2,
					"virtio_net.queue.rx_drops":         3,
					"virtio_net.queue.rx_kicks":         8,
					"virtio_net.queue.rx_packets":       1,
					"virtio_net.queue.rx_xdp_drops":     7,
					"virtio_net.queue.rx_xdp_packets":   4,
					"virtio_net.queue.rx_xdp_redirects": 6,
					"virtio_net.queue.rx_xdp_tx":        5,
					"virtio_net.queue.tx_bytes":         18,
					"virtio_net.queue.tx_kicks":         21,
					"virtio_net.queue.tx_packets":       17,
					"virtio_net.queue.tx_xdp_tx":        19,
					"virtio_net.queue.tx_xdp_tx_drops":  20,
				},
				"queue:12": {
					"virtio_net.queue.rx_bytes":         10,
					"virtio_net.queue.rx_drops":         11,
					"virtio_net.queue.rx_kicks":         16,
					"virtio_net.queue.rx_packets":       9,
					"virtio_net.queue.rx_xdp_drops":     15,
					"virtio_net.queue.rx_xdp_packets":   12,
					"virtio_net.queue.rx_xdp_redirects": 14,
					"virtio_net.queue.rx_xdp_tx":        13,
					"virtio_net.queue.tx_bytes":         24,
					"virtio_net.queue.tx_kicks":         27,
					"virtio_net.queue.tx_packets":       23,
					"virtio_net.queue.tx_xdp_tx":        25,
					"virtio_net.queue.tx_xdp_tx_drops":  26,
				},
			},
		},
		{
			name:  "parses ena",
			iface: "ena_mock",
			want: map[string]map[string]uint64{
				"global": {
					"ena.resume":     3,
					"ena.suspend":    2,
					"ena.tx_timeout": 1,
					"ena.wd_expired": 4,
				},
				"queue:0": {
					"ena.queue.rx_bad_desc_num":     37,
					"ena.queue.rx_bad_req_id":       38,
					"ena.queue.rx_bytes":            29,
					"ena.queue.rx_cnt":              28,
					"ena.queue.rx_csum_good":        31,
					"ena.queue.rx_csum_unchecked":   40,
					"ena.queue.rx_dma_mapping_err":  36,
					"ena.queue.rx_empty_rx_ring":    39,
					"ena.queue.rx_page_alloc_fail":  34,
					"ena.queue.rx_refil_partial":    32,
					"ena.queue.rx_rx_copybreak_pkt": 30,
					"ena.queue.rx_skb_alloc_fail":   35,
					"ena.queue.rx_xdp_aborted":      41,
					"ena.queue.rx_xdp_drop":         42,
					"ena.queue.rx_xdp_invalid":      45,
					"ena.queue.rx_xdp_pass":         43,
					"ena.queue.rx_xdp_redirect":     46,
					"ena.queue.rx_xdp_tx":           44,
					"ena.queue.tx_bad_req_id":       24,
					"ena.queue.tx_bytes":            14,
					"ena.queue.tx_cnt":              13,
					"ena.queue.tx_dma_mapping_err":  17,
					"ena.queue.tx_doorbells":        22,
					"ena.queue.tx_linearize":        18,
					"ena.queue.tx_linearize_failed": 19,
					"ena.queue.tx_llq_buffer_copy":  25,
					"ena.queue.tx_missed_tx":        26,
					"ena.queue.tx_napi_comp":        20,
					"ena.queue.tx_prepare_ctx_err":  23,
					"ena.queue.tx_queue_stop":       15,
					"ena.queue.tx_queue_wakeup":     16,
					"ena.queue.tx_tx_poll":          21,
					"ena.queue.tx_unmask_interrupt": 27,
				},
				"queue:12": {
					"ena.queue.rx_bad_desc_num":     71,
					"ena.queue.rx_bad_req_id":       72,
					"ena.queue.rx_bytes":            63,
					"ena.queue.rx_cnt":              62,
					"ena.queue.rx_csum_good":        65,
					"ena.queue.rx_csum_unchecked":   74,
					"ena.queue.rx_dma_mapping_err":  70,
					"ena.queue.rx_empty_rx_ring":    73,
					"ena.queue.rx_page_alloc_fail":  68,
					"ena.queue.rx_refil_partial":    66,
					"ena.queue.rx_rx_copybreak_pkt": 64,
					"ena.queue.rx_skb_alloc_fail":   69,
					"ena.queue.rx_xdp_aborted":      75,
					"ena.queue.rx_xdp_drop":         76,
					"ena.queue.rx_xdp_invalid":      79,
					"ena.queue.rx_xdp_pass":         77,
					"ena.queue.rx_xdp_redirect":     80,
					"ena.queue.rx_xdp_tx":           78,
					"ena.queue.tx_bad_req_id":       58,
					"ena.queue.tx_bytes":            48,
					"ena.queue.tx_cnt":              47,
					"ena.queue.tx_dma_mapping_err":  51,
					"ena.queue.tx_doorbells":        56,
					"ena.queue.tx_linearize":        52,
					"ena.queue.tx_linearize_failed": 53,
					"ena.queue.tx_llq_buffer_copy":  59,
					"ena.queue.tx_missed_tx":        60,
					"ena.queue.tx_napi_comp":        54,
					"ena.queue.tx_prepare_ctx_err":  57,
					"ena.queue.tx_queue_stop":       49,
					"ena.queue.tx_queue_wakeup":     50,
					"ena.queue.tx_tx_poll":          55,
					"ena.queue.tx_unmask_interrupt": 61,
				},
			},
		},
		{
			name:  "parses mlx5_core",
			iface: "mlx5_core_mock",
			want: map[string]map[string]uint64{
				"global": {
					"mlx5_core.ch_arm":                     100,
					"mlx5_core.ch_eq_rearm":                103,
					"mlx5_core.ch_poll":                    99,
					"mlx5_core.link_down_events_phy":       186,
					"mlx5_core.module_bad_shorted":         200,
					"mlx5_core.module_bus_stuck":           198,
					"mlx5_core.module_high_temp":           199,
					"mlx5_core.rx_bytes":                   2,
					"mlx5_core.rx_crc_errors_phy":          154,
					"mlx5_core.rx_csum_complete":           33,
					"mlx5_core.rx_csum_none":               32,
					"mlx5_core.rx_csum_unnecessary":        31,
					"mlx5_core.rx_discards_phy":            170,
					"mlx5_core.rx_fragments_phy":           174,
					"mlx5_core.rx_in_range_len_errors_phy": 161,
					"mlx5_core.rx_jabbers_phy":             175,
					"mlx5_core.rx_out_of_buffer":           128,
					"mlx5_core.rx_out_of_range_len_phy":    162,
					"mlx5_core.rx_oversize_pkts_buffer":    131,
					"mlx5_core.rx_oversize_pkts_phy":       163,
					"mlx5_core.rx_oversize_pkts_sw_drop":   66,
					"mlx5_core.rx_packets":                 1,
					"mlx5_core.rx_pp_alloc_empty":          80,
					"mlx5_core.rx_steer_missed_packets":    130,
					"mlx5_core.rx_symbol_err_phy":          164,
					"mlx5_core.rx_undersize_pkts_phy":      173,
					"mlx5_core.rx_unsupported_op_phy":      167,
					"mlx5_core.rx_xdp_drop":                37,
					"mlx5_core.rx_xdp_redirect":            38,
					"mlx5_core.rx_xdp_tx_err":              44,
					"mlx5_core.rx_xsk_buff_alloc_err":      118,
					"mlx5_core.tx_bytes":                   4,
					"mlx5_core.tx_discards_phy":            171,
					"mlx5_core.tx_errors_phy":              172,
					"mlx5_core.tx_packets":                 3,
					"mlx5_core.tx_queue_dropped":           50,
					"mlx5_core.tx_queue_stopped":           49,
					"mlx5_core.tx_queue_wake":              54,
					"mlx5_core.tx_xdp_err":                 61,
					"mlx5_core.tx_xsk_err":                 126,
					"mlx5_core.tx_xsk_full":                125,
				},
				"queue:0": {
					"mlx5_core.queue.rx_arfs_err":             244,
					"mlx5_core.queue.rx_buff_alloc_err":       236,
					"mlx5_core.queue.rx_recover":              245,
					"mlx5_core.queue.rx_tls_err":              266,
					"mlx5_core.queue.rx_tls_resync_res_retry": 264,
					"mlx5_core.queue.rx_tls_resync_res_skip":  265,
					"mlx5_core.queue.rx_wqe_err":              232,
					"mlx5_core.queue.rx_xdp_tx_err":           272,
					"mlx5_core.queue.rx_xdp_tx_full":          271,
					"mlx5_core.queue.tx_cqe_err":              363,
					"mlx5_core.queue.tx_dropped":              358,
					"mlx5_core.queue.tx_recover":              360,
				},
				"queue:12": {
					"mlx5_core.queue.rx_arfs_err":             305,
					"mlx5_core.queue.rx_buff_alloc_err":       297,
					"mlx5_core.queue.rx_recover":              306,
					"mlx5_core.queue.rx_tls_err":              327,
					"mlx5_core.queue.rx_tls_resync_res_retry": 325,
					"mlx5_core.queue.rx_tls_resync_res_skip":  326,
					"mlx5_core.queue.rx_wqe_err":              293,
					"mlx5_core.queue.rx_xdp_tx_err":           333,
					"mlx5_core.queue.rx_xdp_tx_full":          332,
					"mlx5_core.queue.tx_cqe_err":              392,
					"mlx5_core.queue.tx_dropped":              387,
					"mlx5_core.queue.tx_recover":              389,
				},
			},
		},
		{
			name:  "parses hv_netvsc",
			iface: "hv_netvsc_mock",
			want: map[string]map[string]uint64{
				"cpu:0": {
					"hv_netvsc.cpu.rx_bytes":      34,
					"hv_netvsc.cpu.rx_packets":    33,
					"hv_netvsc.cpu.tx_bytes":      36,
					"hv_netvsc.cpu.tx_packets":    35,
					"hv_netvsc.cpu.vf_rx_bytes":   38,
					"hv_netvsc.cpu.vf_rx_packets": 37,
					"hv_netvsc.cpu.vf_tx_bytes":   40,
					"hv_netvsc.cpu.vf_tx_packets": 39,
				},
				"cpu:1": {
					"hv_netvsc.cpu.rx_bytes":      42,
					"hv_netvsc.cpu.rx_packets":    41,
					"hv_netvsc.cpu.tx_bytes":      44,
					"hv_netvsc.cpu.tx_packets":    43,
					"hv_netvsc.cpu.vf_rx_bytes":   46,
					"hv_netvsc.cpu.vf_rx_packets": 45,
					"hv_netvsc.cpu.vf_tx_bytes":   48,
					"hv_netvsc.cpu.vf_tx_packets": 47,
				},
				"global": {
					"hv_netvsc.rx_comp_busy": 7,
					"hv_netvsc.rx_no_memory": 8,
					"hv_netvsc.stop_queue":   9,
					"hv_netvsc.tx_busy":      5,
					"hv_netvsc.tx_no_memory": 2,
					"hv_netvsc.tx_no_space":  3,
					"hv_netvsc.tx_scattered": 1,
					"hv_netvsc.tx_send_full": 6,
					"hv_netvsc.tx_too_big":   4,
					"hv_netvsc.wake_queue":   10,
				},
				"queue:0": {
					"hv_netvsc.queue.rx_bytes":    21,
					"hv_netvsc.queue.rx_packets":  20,
					"hv_netvsc.queue.rx_xdp_drop": 22,
					"hv_netvsc.queue.tx_bytes":    18,
					"hv_netvsc.queue.tx_packets":  17,
				},
				"queue:12": {
					"hv_netvsc.queue.rx_bytes":    29,
					"hv_netvsc.queue.rx_packets":  28,
					"hv_netvsc.queue.rx_xdp_drop": 30,
					"hv_netvsc.queue.tx_bytes":    26,
					"hv_netvsc.queue.tx_packets":  25,
				},
			},
		},
		{
			name:  "parses gve",
			iface: "gve_mock",
			want: map[string]map[string]uint64{
				"global": {
					"gve.dma_mapping_error":       15,
					"gve.page_alloc_fail":         14,
					"gve.rx_buf_alloc_fail":       9,
					"gve.rx_desc_err_dropped_pkt": 10,
					"gve.rx_skb_alloc_fail":       8,
					"gve.tx_timeouts":             7,
				},
				"queue:0": {
					"gve.queue.rx_bytes":                  20,
					"gve.queue.rx_completed_desc":         18,
					"gve.queue.rx_copied_pkt":             27,
					"gve.queue.rx_copybreak_pkt":          26,
					"gve.queue.rx_dropped_pkt":            25,
					"gve.queue.rx_drops_invalid_checksum": 31,
					"gve.queue.rx_drops_packet_over_mru":  30,
					"gve.queue.rx_no_buffers_posted":      29,
					"gve.queue.rx_posted_desc":            17,
					"gve.queue.rx_queue_drop_cnt":         28,
					"gve.queue.tx_bytes":                  66,
					"gve.queue.tx_completed_desc":         64,
					"gve.queue.tx_dma_mapping_error":      70,
					"gve.queue.tx_event_counter":          69,
					"gve.queue.tx_posted_desc":            63,
					"gve.queue.tx_stop":                   68,
					"gve.queue.tx_wake":                   67,
				},
				"queue:12": {
					"gve.queue.rx_bytes":                  43,
					"gve.queue.rx_completed_desc":         41,
					"gve.queue.rx_copied_pkt":             50,
					"gve.queue.rx_copybreak_pkt":          49,
					"gve.queue.rx_dropped_pkt":            48,
					"gve.queue.rx_drops_invalid_checksum": 54,
					"gve.queue.rx_drops_packet_over_mru":  53,
					"gve.queue.rx_no_buffers_posted":      52,
					"gve.queue.rx_posted_desc":            40,
					"gve.queue.rx_queue_drop_cnt":         51,
					"gve.queue.tx_bytes":                  79,
					"gve.queue.tx_completed_desc":         77,
					"gve.queue.tx_dma_mapping_error":      83,
					"gve.queue.tx_event_counter":          82,
					"gve.queue.tx_posted_desc":            76,
					"gve.queue.tx_stop":                   81,
					"gve.queue.tx_wake":                   80,
				},
			},
		},
	}

	mockEthtool := new(MockEthtool)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			statsMap, err := mockEthtool.Stats(tc.iface)
			if err != nil {
				t.Errorf("%s not implemented in mock", err)
			}
			iface := strings.ReplaceAll(tc.iface, "_mock", "")
			got := getEthtoolMetrics(iface, statsMap)
			if diff := gocmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ethtool statistics result diff (-want +got):\n%s", diff)
			}
		})
	}
}

type MockCommandRunner struct {
	mock.Mock
}

func (m *MockCommandRunner) FakeRunCommand(cmd []string, _ []string) (string, error) {
	if slices.Contains(cmd, "netstat") {
		return `Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     SYN_SENT
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     SYN_RECV
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT1
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT2
tcp         0      0 46.105.75.4:80          79.220.227.193:2032     TIME_WAIT
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE_WAIT
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     LAST_ACK
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     LISTEN
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     CLOSING
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_SENT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_RECV
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT1
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT2
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     TIME_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LAST_ACK
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LISTEN
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSING
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_SENT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_RECV
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT1
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT2
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     TIME_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LAST_ACK
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LISTEN
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSING
udp         0      0 46.105.75.4:143         90.56.111.177:56867
udp6        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
udp6        0      0 46.105.75.4:143         90.56.111.177:56867
`, nil
	} else if slices.ContainsFunc(cmd, func(s string) bool {
		return strings.Contains(s, "ss")
	}) {
		return `State     Recv-Q    Send-Q    Local Address           Foreign Address
ESTAB     0         0         127.0.0.1:60342         127.0.0.1:46153
TIME-WAIT 0         0         127.0.0.1:46153         127.0.0.1:60342
`, nil
	}
	return `cpu=0 found=27644 invalid=19060 ignore=485633411 insert=0 count=42 drop=1 early_drop=0 max=42 search_restart=39936711
	cpu=1 found=21960 invalid=17288 ignore=475938848 insert=0 count=42 drop=1 early_drop=0 max=42 search_restart=36983181`, nil
}

func createTestNetworkCheck(mockNetStats networkStats) *NetworkCheck {
	return &NetworkCheck{
		net: mockNetStats,
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics:        true,
				WhitelistConntrackMetrics: []string{"max", "count"},
				UseSudoConntrack:          true,
			},
		},
	}
}

func TestDefaultConfiguration(t *testing.T) {
	check := createTestNetworkCheck(nil)
	check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test")

	assert.Equal(t, false, check.config.instance.CollectConnectionState)
	assert.Equal(t, []string(nil), check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "", check.config.instance.ExcludedInterfaceRe)
}

func TestConfiguration(t *testing.T) {
	check := createTestNetworkCheck(nil)
	rawInstanceConfig := []byte(`
collect_connection_state: true
collect_count_metrics: true
excluded_interfaces:
    - eth0
    - lo0
excluded_interface_re: "eth.*"
`)
	err := check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	assert.Nil(t, err)
	assert.Equal(t, true, check.config.instance.CollectConnectionState)
	assert.ElementsMatch(t, []string{"eth0", "lo0"}, check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "eth.*", check.config.instance.ExcludedInterfaceRe)
}

func TestNetworkCheck(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   10,
				BytesSent:   11,
				PacketsRecv: 12,
				Dropin:      13,
				Errin:       14,
				PacketsSent: 15,
				Dropout:     16,
				Errout:      17,
			},
			{
				Name:        "lo0",
				BytesRecv:   18,
				BytesSent:   19,
				PacketsRecv: 20,
				Dropin:      21,
				Errin:       22,
				PacketsSent: 23,
				Dropout:     24,
				Errout:      25,
			},
		},
		netstatAndSnmpCountersValues: map[string]net.ProtoCountersStat{
			"Tcp": {
				Protocol: "Tcp",
				Stats: map[string]int64{
					"RetransSegs":  22,
					"InSegs":       23,
					"OutSegs":      24,
					"ActiveOpens":  39,
					"PassiveOpens": 40,
					"AttemptFails": 41,
					"EstabResets":  42,
					"InErrs":       36,
					"OutRsts":      37,
					"InCsumErrors": 38,
				},
			},
			"Udp": {
				Protocol: "Udp",
				Stats: map[string]int64{
					"InDatagrams":  25,
					"NoPorts":      26,
					"InErrors":     27,
					"OutDatagrams": 28,
					"RcvbufErrors": 29,
					"SndbufErrors": 30,
					"InCsumErrors": 31,
				},
			},
			"TcpExt": {
				Protocol: "TcpExt",
				Stats: map[string]int64{
					"ListenOverflows":      32,
					"ListenDrops":          33,
					"TCPBacklogDrop":       34,
					"TCPRetransFail":       35,
					"IPReversePathFilter":  43,
					"PruneCalled":          44,
					"RcvPruned":            45,
					"OfoPruned":            46,
					"PAWSActive":           47,
					"PAWSEstab":            48,
					"SyncookiesSent":       49,
					"SyncookiesRecv":       50,
					"SyncookiesFailed":     51,
					"TCPAbortOnTimeout":    52,
					"TCPSynRetrans":        53,
					"TCPFromZeroWindowAdv": 54,
					"TCPToZeroWindowAdv":   55,
					"TWRecycled":           56,
				},
			},
		},
		connectionStatsUDP4: []net.ConnectionStat{
			{
				Status: "NONE",
			},
		},
		connectionStatsUDP6: []net.ConnectionStat{
			{
				Status: "NONE",
			},
			{
				Status: "NONE",
			},
		},
		connectionStatsTCP4: []net.ConnectionStat{
			{
				Status: "ESTABLISHED",
			},
			{
				Status: "SYN_SENT",
			},
			{
				Status: "SYN_RECV",
			},
			{
				Status: "FIN_WAIT1",
			},
			{
				Status: "FIN_WAIT2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSE",
			},
			{
				Status: "CLOSE_WAIT",
			},
			{
				Status: "LAST_ACK",
			},
			{
				Status: "LISTEN",
			},
			{
				Status: "CLOSING",
			},
		},

		connectionStatsTCP6: []net.ConnectionStat{
			{
				Status: "ESTABLISHED",
			},
			{
				Status: "SYN_SENT",
			},
			{
				Status: "SYN_RECV",
			},
			{
				Status: "FIN_WAIT1",
			},
			{
				Status: "FIN_WAIT2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSE",
			},
			{
				Status: "CLOSE_WAIT",
			},
			{
				Status: "LAST_ACK",
			},
			{
				Status: "LISTEN",
			},
			{
				Status: "CLOSING",
			},
			{
				Status: "ESTABLISHED",
			},
			{
				Status: "SYN_SENT",
			},
			{
				Status: "SYN_RECV",
			},
			{
				Status: "FIN_WAIT1",
			},
			{
				Status: "FIN_WAIT2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSE",
			},
			{
				Status: "CLOSE_WAIT",
			},
			{
				Status: "LAST_ACK",
			},
			{
				Status: "LISTEN",
			},
			{
				Status: "CLOSING",
			},
		},
	}

	mockEthtool := new(MockEthtool)
	mockEthtool.On("DriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getNewEthtool = func() (ethtoolInterface, error) {
		return mockEthtool, nil
	}

	ssAvailableFunction = func() bool { return false }

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand
	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
collect_connection_state: true
collect_count_metrics: true
collect_ethtool_metrics: true
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/sys/class/net/eth0/speed", []byte(
		`10000`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/sys/class/net/eth0/mtu", []byte(
		`1500`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	var customTags []string

	eth0Tags := []string{"device:eth0", "device_name:eth0", "speed:10000", "mtu:1500"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", lo0Tags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.retrans_segs", float64(22), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(23), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_segs", float64(24), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.active_opens", float64(39), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.passive_opens", float64(40), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.attempt_fails", float64(41), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.established_resets", float64(42), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_errors", float64(36), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_resets", float64(37), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_csum_errors", float64(38), "", customTags)

	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.retrans_segs.count", float64(22), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_segs.count", float64(23), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_segs.count", float64(24), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.active_opens.count", float64(39), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.passive_opens.count", float64(40), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.attempt_fails.count", float64(41), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.established_resets.count", float64(42), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_errors.count", float64(36), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_resets.count", float64(37), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_csum_errors.count", float64(38), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_datagrams", float64(25), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.no_ports", float64(26), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_errors", float64(27), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.out_datagrams", float64(28), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.rcv_buf_errors", float64(29), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.snd_buf_errors", float64(30), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_csum_errors", float64(31), "", customTags)

	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_datagrams.count", float64(25), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.no_ports.count", float64(26), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_errors.count", float64(27), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.out_datagrams.count", float64(28), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.rcv_buf_errors.count", float64(29), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.snd_buf_errors.count", float64(30), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_csum_errors.count", float64(31), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(32), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_drops", float64(33), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(34), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(35), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.ip.reverse_path_filter", float64(43), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.prune_called", float64(44), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.prune_rcv_drops", float64(45), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.prune_ofo_called", float64(46), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.paws_connection_drops", float64(47), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.paws_established_drops", float64(48), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_cookies_sent", float64(49), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_cookies_recv", float64(50), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_cookies_failed", float64(51), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.abort_on_timeout", float64(52), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_retrans", float64(53), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.from_zero_window", float64(54), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.to_zero_window", float64(55), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.tw_reused", float64(56), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.udp4.connections", float64(1), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.udp6.connections", float64(2), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.established", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.opening", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.time_wait", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.closing", float64(6), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.listening", float64(1), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.established", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.opening", float64(4), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.time_wait", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.closing", float64(12), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.listening", float64(2), "", customTags)

	mockSender.AssertCalled(t, "Commit")
}

func TestExcludedInterfaces(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   10,
				BytesSent:   11,
				PacketsRecv: 12,
				Dropin:      13,
				Errin:       14,
				PacketsSent: 15,
				Dropout:     16,
				Errout:      17,
			},
			{
				Name:        "lo0",
				BytesRecv:   18,
				BytesSent:   19,
				PacketsRecv: 20,
				Dropin:      21,
				Errin:       22,
				PacketsSent: 23,
				Dropout:     24,
				Errout:      25,
			},
		},
	}

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
excluded_interfaces:
    - lo0
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/sys/class/net/eth0/speed", []byte(
		`10000`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/sys/class/net/eth0/mtu", []byte(
		`1500`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0", "device_name:eth0", "speed:10000", "mtu:1500"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", lo0Tags)
}

func TestExcludedInterfacesRe(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   10,
				BytesSent:   11,
				PacketsRecv: 12,
				Dropin:      13,
				Errin:       14,
				PacketsSent: 15,
				Dropout:     16,
				Errout:      17,
			},
			{
				Name:        "eth1",
				BytesRecv:   18,
				BytesSent:   19,
				PacketsRecv: 20,
				Dropin:      21,
				Errin:       22,
				PacketsSent: 23,
				Dropout:     24,
				Errout:      25,
			},
			{
				Name:        "lo0",
				BytesRecv:   26,
				BytesSent:   27,
				PacketsRecv: 28,
				Dropin:      29,
				Errin:       30,
				PacketsSent: 31,
				Dropout:     32,
				Errout:      33,
			},
		},
	}

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
excluded_interface_re: "eth[0-9]"
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/sys/class/net/eth0/speed", []byte(
		`10000`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/sys/class/net/eth0/mtu", []byte(
		`1500`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0", "device_name:eth0", "speed:10000", "mtu:1500"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	eth1Tags := []string{"device:eth1", "device_name:eth1"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", eth1Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(26), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(27), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(28), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(29), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(30), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(31), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(32), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(33), "", lo0Tags)
}

func TestFetchEthtoolStats(t *testing.T) {
	mockEthtool := new(MockEthtool)

	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getNewEthtool = func() (ethtoolInterface, error) {
		return mockEthtool, nil
	}

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := createTestNetworkCheck(net)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(`collect_ethtool_metrics: true`), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"device:eth0", "driver_name:ena", "driver_version:mock_version", "queue:0"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.queue.tx_bytes", float64(12345), "", expectedTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.queue.rx_bytes", float64(67890), "", expectedTags)
	expectedTagsCPU := []string{"device:eth0", "driver_name:ena", "driver_version:mock_version", "cpu:0"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.cpu.rx_xdp_tx", float64(123), "", expectedTagsCPU)
	expectedTagsGlobal := []string{"device:eth0", "driver_name:ena", "driver_version:mock_version", "global"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.tx_timeout", float64(456), "", expectedTagsGlobal)
}

func TestFetchEthtoolStatsENOTTY(t *testing.T) {
	mockEthtool := new(MockEthtool)

	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getNewEthtool = func() (ethtoolInterface, error) {
		return mockEthtool, nil
	}

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "virtual_iface",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := createTestNetworkCheck(net)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(`collect_ethtool_metrics: true`), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTagsIfNoError := []string{"device:eth0", "driver_name:ena", "driver_version:mock_version", "queue:0"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.queue.tx_packets", float64(12345), "", expectedTagsIfNoError)
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.queue.rx_packets", float64(67890), "", expectedTagsIfNoError)
	expectedTagsCPUIfNoError := []string{"device:eth0", "driver_name:ena", "driver_version:mock_version", "cpu:0"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.cpu.rx_xdp_tx", float64(123), "", expectedTagsCPUIfNoError)
	expectedTagsGlobal := []string{"device:eth0", "driver_name:ena", "driver_version:mock_version", "global"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.tx_timeout", float64(456), "", expectedTagsGlobal)
}

func TestFetchEthtoolStatsENODEVOnDriverInfo(t *testing.T) {
	mockEthtool := new(MockEthtool)

	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getNewEthtool = func() (ethtoolInterface, error) {
		return mockEthtool, nil
	}

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "enodev_drvinfo_iface",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := createTestNetworkCheck(net)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(`collect_ethtool_metrics: true`), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"device:enodev_drvinfo_iface", "driver_name:ena", "driver_version:mock_version", "queue:0"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.queue.tx_packets", mock.Anything, "", expectedTags)
}

func TestFetchEthtoolStatsENODEVOnStats(t *testing.T) {
	mockEthtool := new(MockEthtool)

	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getNewEthtool = func() (ethtoolInterface, error) {
		return mockEthtool, nil
	}

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "enodev_stats_iface",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := createTestNetworkCheck(net)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(`collect_ethtool_metrics: true`), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"device:enodev_stats_iface", "driver_name:ena", "driver_version:mock_version", "queue:0"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.queue.tx_packets", mock.Anything, "", expectedTags)
}

func TestNetstatAndSnmpCountersUsingCorrectMockedProcfsPath(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)
	var customTags []string

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TcpExt: ListenOverflows ListenDrops TCPBacklogDrop TCPRetransFail
TcpExt: 32 33 34 35
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(32), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_drops", float64(33), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(34), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(35), "", customTags)
}

func TestNetstatAndSnmpCountersWrongConfiguredLocation(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/wrong_mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/wrong_mocked/procfs"
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TcpExt: ListenOverflows ListenDrops TCPBacklogDrop TCPRetransFail
TcpExt: 32 33 34 35
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Equal(t, err, nil)
}

func TestNetstatAndSnmpCountersNoColonFile(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.DebugLvl)
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err = networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`bad file`),
		0644)
	assert.Nil(t, err)

	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, expected ':'")
}

func TestNetstatAndSnmpCountersBadDataLine(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.DebugLvl)
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err = networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TcpExt: `),
		0644)
	assert.Nil(t, err)
	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, not data line")
}

func TestNetstatAndSnmpCountersMismatchedColumns(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.DebugLvl)
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err = networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TcpExt: 1 0 46 79
TcpExt: 32 34 192
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)
	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, expected same number of columns")
}

func TestNetstatAndSnmpCountersLettersForNumbers(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)
	var customTags []string

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TcpExt: 1 0 46 79
TcpExt: ab cd ef gh
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),

		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(32), "", customTags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.listen_drops", float64(33), "", customTags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(34), "", customTags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(35), "", customTags)
}

func TestConntrackMonotonicCount(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
collect_conntrack_metrics: true
conntrack_path: "/usr/bin/conntrack"
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_insert", []byte(
		`13`),
		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"cpu:0"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.conntrack.count", float64(42), "", expectedTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.conntrack.max", float64(42), "", expectedTags)
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.conntrack.ignore_this", mock.Anything, mock.Anything, mock.Anything)
}

func TestConntrackGaugeBlacklist(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
collect_conntrack_metrics: true
conntrack_path: "/usr/bin/conntrack"
whitelist_conntrack_metrics: ["max", "count"]
blacklist_conntrack_metrics: ["count", "entries", "max"]
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_max", []byte(
		`13`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_count", []byte(
		`14`),
		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "Gauge", "system.net.conntrack.max", float64(13), "", []string{})
	mockSender.AssertNotCalled(t, "Gauge", "system.net.conntrack.count", float64(13), "", []string{})
}

func TestConntrackGaugeWhitelist(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
collect_conntrack_metrics: true
conntrack_path: "/usr/bin/conntrack"
whitelist_conntrack_metrics: ["max", "include"]
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_insert", []byte(
		`13`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_include", []byte(
		`14`),
		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "Gauge", "system.net.conntrack.insert", float64(13), "", []string{})
	mockSender.AssertMetric(t, "Gauge", "system.net.conntrack.include", float64(14), "", []string{})
}

func TestFetchQueueStatsSS(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	ssAvailableFunction = func() bool { return true }
	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck := createTestNetworkCheck(net)

	fakeInstanceConfig := []byte(`conntrack_path: ""
collect_connection_state: true
collect_connection_queues: true`)
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, fakeInstanceConfig, []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Histogram", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:established"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:established"})
}

func TestFetchQueueStatsNetstat(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	ssAvailableFunction = func() bool { return false }
	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck := createTestNetworkCheck(net)
	fakeInstanceConfig := []byte(`conntrack_path: ""
collect_connection_state: true
collect_connection_queues: true`)
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, fakeInstanceConfig, []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Histogram", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:established"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:established"})
}

func TestParseSocketStatMetrics(t *testing.T) {
	testcases := []struct {
		name     string
		protocol string
		input    string
		want     map[string]*connectionStateEntry
	}{
		{
			name:     "initializes tcp4 states",
			protocol: "tcp4",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes tcp6 states",
			protocol: "tcp6",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp4 states",
			protocol: "udp4",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp6 states",
			protocol: "udp6",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "collects tcp4 states correctly",
			protocol: "tcp4",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
LISTEN         0           4096           127.0.0.53%lo:53             0.0.0.0:*
LISTEN         1024        0                   0.0.0.0:27500          0.0.0.0:*
LISTEN         0           4096              127.0.0.54:53             0.0.0.0:*
ESTAB          0           0               192.168.64.6:38848    34.107.243.93:443
TIME-WAIT      0           0        192.168.64.6%enp0s1:42804     38.145.32.21:80
`,
			want: map[string]*connectionStateEntry{
				"established": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"opening": emptyConnectionStateEntry(),
				"closing": emptyConnectionStateEntry(),
				"time_wait": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"listening": {
					count: 3,
					recvQ: []uint64{0, 1024, 0},
					sendQ: []uint64{4096, 0, 4096},
				},
			},
		},
		{
			name:     "collects tcp6 states correctly",
			protocol: "tcp6",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
LISTEN         0           4096           127.0.0.53%lo:53             0.0.0.0:*
LISTEN         1024           0                   0.0.0.0:27500          0.0.0.0:*
ESTAB          0           0               192.168.64.6:38848    34.107.243.93:443
TIME-WAIT      0           0        192.168.64.6%enp0s1:42804     38.145.32.21:80
`,
			want: map[string]*connectionStateEntry{
				"established": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"opening": emptyConnectionStateEntry(),
				"closing": emptyConnectionStateEntry(),
				"time_wait": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"listening": {
					count: 2,
					recvQ: []uint64{0, 1024},
					sendQ: []uint64{4096, 0},
				},
			},
		},
		{
			name:     "collects udp4 states correctly",
			protocol: "udp4",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
UNCONN      0           0           127.0.0.53%lo:53             0.0.0.0:*
UNCONN      0           0                   0.0.0.0:27500          0.0.0.0:*
UNCONN      0           0              127.0.0.54:53             0.0.0.0:*
UNCONN      0           0                 0.0.0.0:5355           0.0.0.0:*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 4,
					recvQ: []uint64{0, 0, 0, 0},
					sendQ: []uint64{0, 0, 0, 0},
				},
			},
		},
		{
			name:     "collects udp6 states correctly",
			protocol: "udp6",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
UNCONN      0           0           127.0.0.53%lo:53             0.0.0.0:*
UNCONN      0           0                   0.0.0.0:27500          0.0.0.0:*
UNCONN      0           0              127.0.0.54:53             0.0.0.0:*
UNCONN      0           0                 0.0.0.0:5355           0.0.0.0:*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 4,
					recvQ: []uint64{0, 0, 0, 0},
					sendQ: []uint64{0, 0, 0, 0},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSocketStatsMetrics(tc.protocol, tc.input)
			assert.NoError(t, err)
			if diff := gocmp.Diff(tc.want, got, gocmp.Comparer(connectionStateEntryComparer)); diff != "" {
				t.Errorf("socket statistics result parsing diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseNetstatMetrics(t *testing.T) {
	testcases := []struct {
		name     string
		protocol string
		input    string
		want     map[string]*connectionStateEntry
	}{
		{
			name:     "initializes tcp4 states",
			protocol: "tcp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes tcp6 states",
			protocol: "tcp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp4 states",
			protocol: "udp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp6 states",
			protocol: "udp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "collects tcp4 states correctly",
			protocol: "tcp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        1024      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      1024 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      1024 :::5355                 :::*                    LISTEN
tcp6       1024      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"established": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{1024},
				},
				"opening": emptyConnectionStateEntry(),
				"closing": emptyConnectionStateEntry(),
				"time_wait": {
					count: 1,
					recvQ: []uint64{1024},
					sendQ: []uint64{0},
				},
				"listening": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "collects tcp6 states correctly",
			protocol: "tcp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        1024      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      1024 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      1024 :::5355                 :::*                    LISTEN
tcp6       1024      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening": {
					count: 2,
					recvQ: []uint64{0, 1024},
					sendQ: []uint64{1024, 0},
				},
			},
		},
		{
			name:     "collects udp4 states correctly",
			protocol: "udp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        0      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      0 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      0 :::5355                 :::*                    LISTEN
tcp6       0      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 2,
					recvQ: []uint64{0, 0},
					sendQ: []uint64{0, 0},
				},
			},
		},
		{
			name:     "collects udp6 states correctly",
			protocol: "udp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        0      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      0 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      0 :::5355                 :::*                    LISTEN
tcp6       0      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNetstatMetrics(tc.protocol, tc.input)
			assert.NoError(t, err)
			if diff := gocmp.Diff(tc.want, got, gocmp.Comparer(connectionStateEntryComparer)); diff != "" {
				t.Errorf("netstat result parsing diff (-want +got):\n%s", diff)
			}
		})
	}
}

func connectionStateEntryComparer(a, b *connectionStateEntry) bool {
	return a.count == b.count &&
		gocmp.Equal(a.recvQ, b.recvQ) &&
		gocmp.Equal(a.sendQ, b.sendQ)
}

func emptyConnectionStateEntry() *connectionStateEntry {
	return &connectionStateEntry{
		count: 0,
		recvQ: []uint64{},
		sendQ: []uint64{},
	}
}
