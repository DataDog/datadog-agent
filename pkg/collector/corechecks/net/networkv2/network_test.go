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

	return ethtool.DrvInfo{}, unix.ENOTTY
}

func (f *MockEthtool) Stats(iface string) (map[string]uint64, error) {
	if iface == "eth0" {
		return map[string]uint64{
			"queue_0_tx_packets": 12345,
			"rx_packets[0]":      67890,
			"cpu0_rx_xdp_tx":     123,
			"tx_timeout":         456,
			"tx_queue_dropped":   789, // Tests queue name parsing
		}, nil
	}
	if iface == "gve_mock" {
		return map[string]uint64{
			"rx_packets":                       7846662,
			"tx_packets":                       3471873,
			"rx_bytes":                         7321238570,
			"tx_bytes":                         5434567485,
			"rx_dropped":                       0,
			"tx_dropped":                       0,
			"tx_timeouts":                      0,
			"rx_skb_alloc_fail":                0,
			"rx_buf_alloc_fail":                0,
			"rx_desc_err_dropped_pkt":          0,
			"interface_up_cnt":                 3,
			"interface_down_cnt":               2,
			"reset_cnt":                        0,
			"page_alloc_fail":                  0,
			"dma_mapping_error":                0,
			"stats_report_trigger_cnt":         0,
			"rx_posted_desc[0]":                308870,
			"rx_completed_desc[0]":             307847,
			"rx_consumed_desc[0]":              1023,
			"rx_bytes[0]":                      237708080,
			"rx_cont_packet_cnt[0]":            0,
			"rx_frag_flip_cnt[0]":              0,
			"rx_frag_copy_cnt[0]":              0,
			"rx_frag_alloc_cnt[0]":             0,
			"rx_dropped_pkt[0]":                0,
			"rx_copybreak_pkt[0]":              111135,
			"rx_copied_pkt[0]":                 111135,
			"rx_queue_drop_cnt[0]":             0,
			"rx_no_buffers_posted[0]":          0,
			"rx_drops_packet_over_mru[0]":      0,
			"rx_drops_invalid_checksum[0]":     0,
			"rx_xdp_aborted[0]":                0,
			"rx_xdp_drop[0]":                   0,
			"rx_xdp_pass[0]":                   0,
			"rx_xdp_tx[0]":                     0,
			"rx_xdp_redirect[0]":               0,
			"rx_xdp_tx_errors[0]":              0,
			"rx_xdp_redirect_errors[0]":        0,
			"rx_xdp_alloc_fails[0]":            0,
			"rx_posted_desc[1]":                265032,
			"rx_completed_desc[1]":             264009,
			"rx_consumed_desc[1]":              1023,
			"rx_bytes[1]":                      170824474,
			"rx_cont_packet_cnt[1]":            0,
			"rx_frag_flip_cnt[1]":              0,
			"rx_frag_copy_cnt[1]":              0,
			"rx_frag_alloc_cnt[1]":             0,
			"rx_dropped_pkt[1]":                0,
			"rx_copybreak_pkt[1]":              123328,
			"rx_copied_pkt[1]":                 123328,
			"rx_queue_drop_cnt[1]":             0,
			"rx_no_buffers_posted[1]":          0,
			"rx_drops_packet_over_mru[1]":      0,
			"rx_drops_invalid_checksum[1]":     0,
			"rx_xdp_aborted[1]":                0,
			"rx_xdp_drop[1]":                   0,
			"rx_xdp_pass[1]":                   0,
			"rx_xdp_tx[1]":                     0,
			"rx_xdp_redirect[1]":               0,
			"rx_xdp_tx_errors[1]":              0,
			"rx_xdp_redirect_errors[1]":        0,
			"rx_xdp_alloc_fails[1]":            0,
			"tx_posted_desc[0]":                0,
			"tx_completed_desc[0]":             0,
			"tx_consumed_desc[0]":              71,
			"tx_bytes[0]":                      239878179,
			"tx_wake[0]":                       0,
			"tx_stop[0]":                       0,
			"tx_event_counter[0]":              0,
			"tx_dma_mapping_error[0]":          0,
			"tx_xsk_wakeup[0]":                 0,
			"tx_xsk_done[0]":                   0,
			"tx_xsk_sent[0]":                   0,
			"tx_xdp_xmit[0]":                   0,
			"tx_xdp_xmit_errors[0]":            0,
			"tx_posted_desc[1]":                0,
			"tx_completed_desc[1]":             0,
			"tx_consumed_desc[1]":              197,
			"tx_bytes[1]":                      328688757,
			"tx_wake[1]":                       0,
			"tx_stop[1]":                       0,
			"tx_event_counter[1]":              0,
			"tx_dma_mapping_error[1]":          0,
			"tx_xsk_wakeup[1]":                 0,
			"tx_xsk_done[1]":                   0,
			"tx_xsk_sent[1]":                   0,
			"tx_xdp_xmit[1]":                   0,
			"tx_xdp_xmit_errors[1]":            0,
			"adminq_prod_cnt":                  166,
			"adminq_cmd_fail":                  0,
			"adminq_timeouts":                  0,
			"adminq_describe_device_cnt":       1,
			"adminq_cfg_device_resources_cnt":  1,
			"adminq_register_page_list_cnt":    0,
			"adminq_unregister_page_list_cnt":  0,
			"adminq_create_tx_queue_cnt":       48,
			"adminq_create_rx_queue_cnt":       48,
			"adminq_destroy_tx_queue_cnt":      32,
			"adminq_destroy_rx_queue_cnt":      32,
			"adminq_dcfg_device_resources_cnt": 0,
			"adminq_set_driver_parameter_cnt":  0,
			"adminq_report_stats_cnt":          1,
			"adminq_report_link_speed_cnt":     1,
		}, nil
	}

	if iface == "mlx5_core_mock" {
		return map[string]uint64{
			"rx_packets":                      7102787,
			"rx_bytes":                        7734170528,
			"tx_packets":                      6567017,
			"tx_bytes":                        6600917380,
			"tx_tso_packets":                  932884,
			"tx_tso_bytes":                    5937521781,
			"tx_tso_inner_packets":            0,
			"tx_tso_inner_bytes":              0,
			"tx_added_vlan_packets":           0,
			"tx_nop":                          714571,
			"tx_mpwqe_blks":                   1741628,
			"tx_mpwqe_pkts":                   1742087,
			"tx_tls_encrypted_packets":        0,
			"tx_tls_encrypted_bytes":          0,
			"tx_tls_ooo":                      0,
			"tx_tls_dump_packets":             0,
			"tx_tls_dump_bytes":               0,
			"tx_tls_resync_bytes":             0,
			"tx_tls_skip_no_sync_data":        0,
			"tx_tls_drop_no_sync_data":        0,
			"tx_tls_drop_bypass_req":          0,
			"rx_lro_packets":                  0,
			"rx_lro_bytes":                    0,
			"rx_gro_packets":                  0,
			"rx_gro_bytes":                    0,
			"rx_gro_skbs":                     0,
			"rx_gro_match_packets":            0,
			"rx_gro_large_hds":                0,
			"rx_ecn_mark":                     0,
			"rx_removed_vlan_packets":         0,
			"rx_csum_unnecessary":             10075,
			"rx_csum_none":                    107216,
			"rx_csum_complete":                6985496,
			"rx_csum_complete_tail":           0,
			"rx_csum_complete_tail_slow":      0,
			"rx_csum_unnecessary_inner":       0,
			"rx_xdp_drop":                     0,
			"rx_xdp_redirect":                 0,
			"rx_xdp_tx_xmit":                  0,
			"rx_xdp_tx_mpwqe":                 0,
			"rx_xdp_tx_inlnw":                 0,
			"rx_xdp_tx_nops":                  0,
			"rx_xdp_tx_full":                  0,
			"rx_xdp_tx_err":                   0,
			"rx_xdp_tx_cqe":                   0,
			"tx_csum_none":                    395806,
			"tx_csum_partial":                 2879770,
			"tx_csum_partial_inner":           0,
			"tx_queue_stopped":                0,
			"tx_queue_dropped":                0,
			"tx_xmit_more":                    479,
			"tx_recover":                      0,
			"tx_cqes":                         3275097,
			"tx_queue_wake":                   0,
			"tx_cqe_err":                      0,
			"tx_xdp_xmit":                     0,
			"tx_xdp_mpwqe":                    0,
			"tx_xdp_inlnw":                    0,
			"tx_xdp_nops":                     0,
			"tx_xdp_full":                     0,
			"tx_xdp_err":                      0,
			"tx_xdp_cqes":                     0,
			"rx_wqe_err":                      0,
			"rx_mpwqe_filler_cqes":            0,
			"rx_mpwqe_filler_strides":         0,
			"rx_oversize_pkts_sw_drop":        0,
			"rx_buff_alloc_err":               0,
			"rx_cqe_compress_blks":            0,
			"rx_cqe_compress_pkts":            0,
			"rx_congst_umr":                   8,
			"rx_arfs_add":                     0,
			"rx_arfs_request_in":              0,
			"rx_arfs_request_out":             0,
			"rx_arfs_expired":                 0,
			"rx_arfs_err":                     0,
			"rx_recover":                      0,
			"rx_pp_alloc_fast":                3574227,
			"rx_pp_alloc_slow":                1008,
			"rx_pp_alloc_slow_high_order":     0,
			"rx_pp_alloc_empty":               1008,
			"rx_pp_alloc_refill":              7997,
			"rx_pp_alloc_waive":               0,
			"rx_pp_recycle_cached":            3008813,
			"rx_pp_recycle_cache_full":        277864,
			"rx_pp_recycle_ring":              536556,
			"rx_pp_recycle_ring_full":         5025,
			"rx_pp_recycle_released_ref":      0,
			"rx_tls_decrypted_packets":        0,
			"rx_tls_decrypted_bytes":          0,
			"rx_tls_resync_req_pkt":           0,
			"rx_tls_resync_req_start":         0,
			"rx_tls_resync_req_end":           0,
			"rx_tls_resync_req_skip":          0,
			"rx_tls_resync_res_ok":            0,
			"rx_tls_resync_res_retry":         0,
			"rx_tls_resync_res_skip":          0,
			"rx_tls_err":                      0,
			"ch_events":                       3977176,
			"ch_poll":                         4037687,
			"ch_arm":                          3809860,
			"ch_aff_change":                   0,
			"ch_force_irq":                    0,
			"ch_eq_rearm":                     0,
			"rx_xsk_packets":                  0,
			"rx_xsk_bytes":                    0,
			"rx_xsk_csum_complete":            0,
			"rx_xsk_csum_unnecessary":         0,
			"rx_xsk_csum_unnecessary_inner":   0,
			"rx_xsk_csum_none":                0,
			"rx_xsk_ecn_mark":                 0,
			"rx_xsk_removed_vlan_packets":     0,
			"rx_xsk_xdp_drop":                 0,
			"rx_xsk_xdp_redirect":             0,
			"rx_xsk_wqe_err":                  0,
			"rx_xsk_mpwqe_filler_cqes":        0,
			"rx_xsk_mpwqe_filler_strides":     0,
			"rx_xsk_oversize_pkts_sw_drop":    0,
			"rx_xsk_buff_alloc_err":           0,
			"rx_xsk_cqe_compress_blks":        0,
			"rx_xsk_cqe_compress_pkts":        0,
			"rx_xsk_congst_umr":               0,
			"tx_xsk_xmit":                     0,
			"tx_xsk_mpwqe":                    0,
			"tx_xsk_inlnw":                    0,
			"tx_xsk_full":                     0,
			"tx_xsk_err":                      0,
			"tx_xsk_cqes":                     0,
			"rx_out_of_buffer":                0,
			"rx_if_down_packets":              0,
			"rx_steer_missed_packets":         0,
			"rx_oversize_pkts_buffer":         0,
			"rx_vport_unicast_packets":        7102787,
			"rx_vport_unicast_bytes":          7734170528,
			"tx_vport_unicast_packets":        6533830,
			"tx_vport_unicast_bytes":          6598924136,
			"rx_vport_multicast_packets":      0,
			"rx_vport_multicast_bytes":        0,
			"tx_vport_multicast_packets":      55,
			"tx_vport_multicast_bytes":        4102,
			"rx_vport_broadcast_packets":      0,
			"rx_vport_broadcast_bytes":        0,
			"tx_vport_broadcast_packets":      33132,
			"tx_vport_broadcast_bytes":        1989142,
			"rx_vport_rdma_unicast_packets":   0,
			"rx_vport_rdma_unicast_bytes":     0,
			"tx_vport_rdma_unicast_packets":   0,
			"tx_vport_rdma_unicast_bytes":     0,
			"rx_vport_rdma_multicast_packets": 0,
			"rx_vport_rdma_multicast_bytes":   0,
			"tx_vport_rdma_multicast_packets": 0,
			"tx_vport_rdma_multicast_bytes":   0,
			"tx_packets_phy":                  0,
			"rx_packets_phy":                  0,
			"rx_crc_errors_phy":               0,
			"tx_bytes_phy":                    0,
			"rx_bytes_phy":                    0,
			"tx_multicast_phy":                0,
			"tx_broadcast_phy":                0,
			"rx_multicast_phy":                0,
			"rx_broadcast_phy":                0,
			"rx_in_range_len_errors_phy":      0,
			"rx_out_of_range_len_phy":         0,
			"rx_oversize_pkts_phy":            0,
			"rx_symbol_err_phy":               0,
			"tx_mac_control_phy":              0,
			"rx_mac_control_phy":              0,
			"rx_unsupported_op_phy":           0,
			"rx_pause_ctrl_phy":               0,
			"tx_pause_ctrl_phy":               0,
			"rx_discards_phy":                 0,
			"tx_discards_phy":                 0,
			"tx_errors_phy":                   0,
			"rx_undersize_pkts_phy":           0,
			"rx_fragments_phy":                0,
			"rx_jabbers_phy":                  0,
			"rx_64_bytes_phy":                 0,
			"rx_65_to_127_bytes_phy":          0,
			"rx_128_to_255_bytes_phy":         0,
			"rx_256_to_511_bytes_phy":         0,
			"rx_512_to_1023_bytes_phy":        0,
			"rx_1024_to_1518_bytes_phy":       0,
			"rx_1519_to_2047_bytes_phy":       0,
			"rx_2048_to_4095_bytes_phy":       0,
			"rx_4096_to_8191_bytes_phy":       0,
			"rx_8192_to_10239_bytes_phy":      0,
			"link_down_events_phy":            0,
			"rx_prio0_bytes":                  0,
			"rx_prio0_packets":                0,
			"rx_prio0_discards":               0,
			"tx_prio0_bytes":                  0,
			"tx_prio0_packets":                0,
			"rx_prio1_bytes":                  0,
			"rx_prio1_packets":                0,
			"rx_prio1_discards":               0,
			"tx_prio1_bytes":                  0,
			"tx_prio1_packets":                0,
			"module_unplug":                   0,
			"module_bus_stuck":                0,
			"module_high_temp":                0,
			"module_bad_shorted":              0,
			"ch0_events":                      576772,
			"ch0_poll":                        587661,
			"ch0_arm":                         553404,
			"ch0_aff_change":                  0,
			"ch0_force_irq":                   0,
			"ch0_eq_rearm":                    0,
			"ch1_events":                      440140,
			"ch1_poll":                        448083,
			"ch1_arm":                         419037,
			"ch1_aff_change":                  0,
			"ch1_force_irq":                   0,
			"ch1_eq_rearm":                    0,
			"rx0_packets":                     1089603,
			"rx0_bytes":                       1145880978,
			"rx0_csum_complete":               981151,
			"rx0_csum_complete_tail":          0,
			"rx0_csum_complete_tail_slow":     0,
			"rx0_csum_unnecessary":            1236,
			"rx0_csum_unnecessary_inner":      0,
			"rx0_csum_none":                   107216,
			"rx0_xdp_drop":                    0,
			"rx0_xdp_redirect":                0,
			"rx0_lro_packets":                 0,
			"rx0_lro_bytes":                   0,
			"rx0_gro_packets":                 0,
			"rx0_gro_bytes":                   0,
			"rx0_gro_skbs":                    0,
			"rx0_gro_match_packets":           0,
			"rx0_gro_large_hds":               0,
			"rx0_ecn_mark":                    0,
			"rx0_removed_vlan_packets":        0,
			"rx0_wqe_err":                     0,
			"rx0_mpwqe_filler_cqes":           0,
			"rx0_mpwqe_filler_strides":        0,
			"rx0_oversize_pkts_sw_drop":       0,
			"rx0_buff_alloc_err":              0,
			"rx0_cqe_compress_blks":           0,
			"rx0_cqe_compress_pkts":           0,
			"rx0_congst_umr":                  0,
			"rx0_arfs_add":                    0,
			"rx0_arfs_request_in":             0,
			"rx0_arfs_request_out":            0,
			"rx0_arfs_expired":                0,
			"rx0_arfs_err":                    0,
			"rx0_recover":                     0,
			"rx0_pp_alloc_fast":               547007,
			"rx0_pp_alloc_slow":               128,
			"rx0_pp_alloc_slow_high_order":    0,
			"rx0_pp_alloc_empty":              128,
			"rx0_pp_alloc_refill":             1665,
			"rx0_pp_alloc_waive":              0,
			"rx0_pp_recycle_cached":           434366,
			"rx0_pp_recycle_cache_full":       58129,
			"rx0_pp_recycle_ring":             110336,
			"rx0_pp_recycle_ring_full":        1,
			"rx0_pp_recycle_released_ref":     0,
			"rx0_tls_decrypted_packets":       0,
			"rx0_tls_decrypted_bytes":         0,
			"rx0_tls_resync_req_pkt":          0,
			"rx0_tls_resync_req_start":        0,
			"rx0_tls_resync_req_end":          0,
			"rx0_tls_resync_req_skip":         0,
			"rx0_tls_resync_res_ok":           0,
			"rx0_tls_resync_res_retry":        0,
			"rx0_tls_resync_res_skip":         0,
			"rx0_tls_err":                     0,
			"rx0_xdp_tx_xmit":                 0,
			"rx0_xdp_tx_mpwqe":                0,
			"rx0_xdp_tx_inlnw":                0,
			"rx0_xdp_tx_nops":                 0,
			"rx0_xdp_tx_full":                 0,
			"rx0_xdp_tx_err":                  0,
			"rx0_xdp_tx_cqes":                 0,
			"rx1_packets":                     918923,
			"rx1_bytes":                       1049366856,
			"rx1_csum_complete":               917714,
			"rx1_csum_complete_tail":          0,
			"rx1_csum_complete_tail_slow":     0,
			"rx1_csum_unnecessary":            1209,
			"rx1_csum_unnecessary_inner":      0,
			"rx1_csum_none":                   0,
			"rx1_xdp_drop":                    0,
			"rx1_xdp_redirect":                0,
			"rx1_lro_packets":                 0,
			"rx1_lro_bytes":                   0,
			"rx1_gro_packets":                 0,
			"rx1_gro_bytes":                   0,
			"rx1_gro_skbs":                    0,
			"rx1_gro_match_packets":           0,
			"rx1_gro_large_hds":               0,
			"rx1_ecn_mark":                    0,
			"rx1_removed_vlan_packets":        0,
			"rx1_wqe_err":                     0,
			"rx1_mpwqe_filler_cqes":           0,
			"rx1_mpwqe_filler_strides":        0,
			"rx1_oversize_pkts_sw_drop":       0,
			"rx1_buff_alloc_err":              0,
			"rx1_cqe_compress_blks":           0,
			"rx1_cqe_compress_pkts":           0,
			"rx1_congst_umr":                  0,
			"rx1_arfs_add":                    0,
			"rx1_arfs_request_in":             0,
			"rx1_arfs_request_out":            0,
			"rx1_arfs_expired":                0,
			"rx1_arfs_err":                    0,
			"rx1_recover":                     0,
			"rx1_pp_alloc_fast":               462146,
			"rx1_pp_alloc_slow":               101,
			"rx1_pp_alloc_slow_high_order":    0,
			"rx1_pp_alloc_empty":              101,
			"rx1_pp_alloc_refill":             1177,
			"rx1_pp_alloc_waive":              0,
			"rx1_pp_recycle_cached":           381805,
			"rx1_pp_recycle_cache_full":       46263,
			"rx1_pp_recycle_ring":             77521,
			"rx1_pp_recycle_ring_full":        0,
			"rx1_pp_recycle_released_ref":     0,
			"rx1_tls_decrypted_packets":       0,
			"rx1_tls_decrypted_bytes":         0,
			"rx1_tls_resync_req_pkt":          0,
			"rx1_tls_resync_req_start":        0,
			"rx1_tls_resync_req_end":          0,
			"rx1_tls_resync_req_skip":         0,
			"rx1_tls_resync_res_ok":           0,
			"rx1_tls_resync_res_retry":        0,
			"rx1_tls_resync_res_skip":         0,
			"rx1_tls_err":                     0,
			"rx1_xdp_tx_xmit":                 0,
			"rx1_xdp_tx_mpwqe":                0,
			"rx1_xdp_tx_inlnw":                0,
			"rx1_xdp_tx_nops":                 0,
			"rx1_xdp_tx_full":                 0,
			"rx1_xdp_tx_err":                  0,
			"rx1_xdp_tx_cqes":                 0,
			"tx0_packets":                     882942,
			"tx0_bytes":                       767148532,
			"tx0_tso_packets":                 127402,
			"tx0_tso_bytes":                   677790711,
			"tx0_tso_inner_packets":           0,
			"tx0_tso_inner_bytes":             0,
			"tx0_csum_partial":                384251,
			"tx0_csum_partial_inner":          0,
			"tx0_added_vlan_packets":          0,
			"tx0_nop":                         118883,
			"tx0_mpwqe_blks":                  317152,
			"tx0_mpwqe_pkts":                  317152,
			"tx0_tls_encrypted_packets":       0,
			"tx0_tls_encrypted_bytes":         0,
			"tx0_tls_ooo":                     0,
			"tx0_tls_dump_packets":            0,
			"tx0_tls_dump_bytes":              0,
			"tx0_tls_resync_bytes":            0,
			"tx0_tls_skip_no_sync_data":       0,
			"tx0_tls_drop_no_sync_data":       0,
			"tx0_tls_drop_bypass_req":         0,
			"tx0_csum_none":                   141873,
			"tx0_stopped":                     0,
			"tx0_dropped":                     0,
			"tx0_xmit_more":                   0,
			"tx0_recover":                     0,
			"tx0_cqes":                        526124,
			"tx0_wake":                        0,
			"tx0_cqe_err":                     0,
			"tx1_packets":                     783532,
			"tx1_bytes":                       843782693,
			"tx1_tso_packets":                 117264,
			"tx1_tso_bytes":                   764933447,
			"tx1_tso_inner_packets":           0,
			"tx1_tso_inner_bytes":             0,
			"tx1_csum_partial":                325312,
			"tx1_csum_partial_inner":          0,
			"tx1_added_vlan_packets":          0,
			"tx1_nop":                         72965,
			"tx1_mpwqe_blks":                  172245,
			"tx1_mpwqe_pkts":                  172245,
			"tx1_tls_encrypted_packets":       0,
			"tx1_tls_encrypted_bytes":         0,
			"tx1_tls_ooo":                     0,
			"tx1_tls_dump_packets":            0,
			"tx1_tls_dump_bytes":              0,
			"tx1_tls_resync_bytes":            0,
			"tx1_tls_skip_no_sync_data":       0,
			"tx1_tls_drop_no_sync_data":       0,
			"tx1_tls_drop_bypass_req":         0,
			"tx1_csum_none":                   33212,
			"tx1_stopped":                     0,
			"tx1_dropped":                     0,
			"tx1_xmit_more":                   1,
			"tx1_recover":                     0,
			"tx1_cqes":                        358523,
			"tx1_wake":                        0,
			"tx1_cqe_err":                     0,
			"tx0_xdp_xmit":                    0,
			"tx0_xdp_mpwqe":                   0,
			"tx0_xdp_inlnw":                   0,
			"tx0_xdp_nops":                    0,
			"tx0_xdp_full":                    0,
			"tx0_xdp_err":                     0,
			"tx0_xdp_cqes":                    0,
			"tx1_xdp_xmit":                    0,
			"tx1_xdp_mpwqe":                   0,
			"tx1_xdp_inlnw":                   0,
			"tx1_xdp_nops":                    0,
			"tx1_xdp_full":                    0,
			"tx1_xdp_err":                     0,
			"tx1_xdp_cqes":                    0,
		}, nil
	}

	if iface == "hv_netvsc_mock" {
		return map[string]uint64{
			"tx_scattered":            0,
			"tx_no_memory":            0,
			"tx_no_space":             0,
			"tx_too_big":              0,
			"tx_busy":                 0,
			"tx_send_full":            0,
			"rx_comp_busy":            0,
			"rx_no_memory":            0,
			"stop_queue":              0,
			"wake_queue":              0,
			"vlan_error":              0,
			"vf_rx_packets":           6623621,
			"vf_rx_bytes":             14605106242,
			"vf_tx_packets":           5456569,
			"vf_tx_bytes":             3121069663,
			"vf_tx_dropped":           0,
			"tx_queue_0_packets":      0,
			"tx_queue_0_bytes":        0,
			"tx_queue_0_xdp_xmit":     0,
			"rx_queue_0_packets":      0,
			"rx_queue_0_bytes":        0,
			"rx_queue_0_xdp_drop":     0,
			"rx_queue_0_xdp_redirect": 0,
			"rx_queue_0_xdp_tx":       0,
			"tx_queue_1_packets":      0,
			"tx_queue_1_bytes":        0,
			"tx_queue_1_xdp_xmit":     0,
			"rx_queue_1_packets":      0,
			"rx_queue_1_bytes":        0,
			"rx_queue_1_xdp_drop":     0,
			"rx_queue_1_xdp_redirect": 0,
			"rx_queue_1_xdp_tx":       0,
			"vlan_error":              0,
			"cpu0_rx_packets":         842180,
			"cpu0_rx_bytes":           1887367874,
			"cpu0_tx_packets":         691068,
			"cpu0_tx_bytes":           383548096,
			"cpu0_vf_rx_packets":      842180,
			"cpu0_vf_rx_bytes":        1887367874,
			"cpu0_vf_tx_packets":      691068,
			"cpu0_vf_tx_bytes":        383548096,
			"cpu1_rx_packets":         837228,
			"cpu1_rx_bytes":           2017958880,
			"cpu1_tx_packets":         683298,
			"cpu1_tx_bytes":           348690534,
			"cpu1_vf_rx_packets":      837228,
			"cpu1_vf_rx_bytes":        2017958880,
			"cpu1_vf_tx_packets":      683298,
			"cpu1_vf_tx_bytes":        348690534,
		}, nil
	}

	if iface == "virtio_net_mock" {
		return map[string]uint64{
			"rx_queue_0_packets":       1974584,
			"rx_queue_0_bytes":         581561073,
			"rx_queue_0_drops":         0,
			"rx_queue_0_xdp_packets":   0,
			"rx_queue_0_xdp_tx":        0,
			"rx_queue_0_xdp_redirects": 0,
			"rx_queue_0_xdp_drops":     0,
			"rx_queue_0_kicks":         32,
			"rx_queue_1_packets":       5594925,
			"rx_queue_1_bytes":         1707162546,
			"rx_queue_1_drops":         0,
			"rx_queue_1_xdp_packets":   0,
			"rx_queue_1_xdp_tx":        0,
			"rx_queue_1_xdp_redirects": 0,
			"rx_queue_1_xdp_drops":     0,
			"rx_queue_1_kicks":         90,
			"tx_queue_0_packets":       1293809,
			"tx_queue_0_bytes":         787537066,
			"tx_queue_0_xdp_tx":        0,
			"tx_queue_0_xdp_tx_drops":  0,
			"tx_queue_0_kicks":         20,
			"tx_queue_0_tx_timeouts":   0,
			"tx_queue_1_packets":       4687786,
			"tx_queue_1_bytes":         1013540122,
			"tx_queue_1_xdp_tx":        0,
			"tx_queue_1_xdp_tx_drops":  0,
			"tx_queue_1_kicks":         72,
			"tx_queue_1_tx_timeouts":   0,
		}, nil
	}

	if iface == "ena_mock" {
		return map[string]uint64{
			"tx_timeout":                   0,
			"suspend":                      0,
			"resume":                       0,
			"wd_expired":                   0,
			"interface_up":                 2,
			"interface_down":               1,
			"admin_q_pause":                0,
			"bw_in_allowance_exceeded":     0,
			"bw_out_allowance_exceeded":    54,
			"pps_allowance_exceeded":       0,
			"conntrack_allowance_exceeded": 0,
			"linklocal_allowance_exceeded": 0,
			"queue_0_tx_cnt":               337803,
			"queue_0_tx_bytes":             1219555904,
			"queue_0_tx_queue_stop":        0,
			"queue_0_tx_queue_wakeup":      0,
			"queue_0_tx_dma_mapping_err":   0,
			"queue_0_tx_linearize":         0,
			"queue_0_tx_linearize_failed":  0,
			"queue_0_tx_napi_comp":         427401,
			"queue_0_tx_tx_poll":           427638,
			"queue_0_tx_doorbells":         252614,
			"queue_0_tx_prepare_ctx_err":   0,
			"queue_0_tx_bad_req_id":        0,
			"queue_0_tx_llq_buffer_copy":   227282,
			"queue_0_tx_missed_tx":         0,
			"queue_0_tx_unmask_interrupt":  427401,
			"queue_0_rx_cnt":               522142,
			"queue_0_rx_bytes":             536299914,
			"queue_0_rx_rx_copybreak_pkt":  173256,
			"queue_0_rx_csum_good":         464760,
			"queue_0_rx_refil_partial":     0,
			"queue_0_rx_csum_bad":          0,
			"queue_0_rx_page_alloc_fail":   0,
			"queue_0_rx_skb_alloc_fail":    0,
			"queue_0_rx_dma_mapping_err":   0,
			"queue_0_rx_bad_desc_num":      0,
			"queue_0_rx_bad_req_id":        0,
			"queue_0_rx_empty_rx_ring":     0,
			"queue_0_rx_csum_unchecked":    24836,
			"queue_0_rx_xdp_aborted":       0,
			"queue_0_rx_xdp_drop":          0,
			"queue_0_rx_xdp_pass":          0,
			"queue_0_rx_xdp_tx":            0,
			"queue_0_rx_xdp_invalid":       0,
			"queue_0_rx_xdp_redirect":      0,
			"queue_1_tx_cnt":               316261,
			"queue_1_tx_bytes":             1081020015,
			"queue_1_tx_queue_stop":        0,
			"queue_1_tx_queue_wakeup":      0,
			"queue_1_tx_dma_mapping_err":   0,
			"queue_1_tx_linearize":         0,
			"queue_1_tx_linearize_failed":  0,
			"queue_1_tx_napi_comp":         398895,
			"queue_1_tx_tx_poll":           398945,
			"queue_1_tx_doorbells":         237193,
			"queue_1_tx_prepare_ctx_err":   0,
			"queue_1_tx_bad_req_id":        0,
			"queue_1_tx_llq_buffer_copy":   201610,
			"queue_1_tx_missed_tx":         0,
			"queue_1_tx_unmask_interrupt":  398895,
			"queue_1_rx_cnt":               473892,
			"queue_1_rx_bytes":             506687642,
			"queue_1_rx_rx_copybreak_pkt":  132199,
			"queue_1_rx_csum_good":         469723,
			"queue_1_rx_refil_partial":     0,
			"queue_1_rx_csum_bad":          0,
			"queue_1_rx_page_alloc_fail":   0,
			"queue_1_rx_skb_alloc_fail":    0,
			"queue_1_rx_dma_mapping_err":   0,
			"queue_1_rx_bad_desc_num":      0,
			"queue_1_rx_bad_req_id":        0,
			"queue_1_rx_empty_rx_ring":     0,
			"queue_1_rx_csum_unchecked":    4,
			"queue_1_rx_xdp_aborted":       0,
			"queue_1_rx_xdp_drop":          0,
			"queue_1_rx_xdp_pass":          0,
			"queue_1_rx_xdp_tx":            0,
			"queue_1_rx_xdp_invalid":       0,
			"queue_1_rx_xdp_redirect":      0,
			"ena_admin_q_aborted_cmd":      0,
			"ena_admin_q_submitted_cmd":    2687,
			"ena_admin_q_completed_cmd":    2687,
			"ena_admin_q_out_of_space":     0,
		}, nil
	}

	if iface == "veth_mock" {
		return map[string]uint64{
			"peer_ifindex":               3,
			"rx_queue_0_xdp_packets":     0,
			"rx_queue_0_xdp_bytes":       0,
			"rx_queue_0_drops":           0,
			"rx_queue_0_xdp_redirect":    0,
			"rx_queue_0_xdp_drops":       0,
			"rx_queue_0_xdp_tx":          0,
			"rx_queue_0_xdp_tx_errors":   0,
			"tx_queue_0_xdp_xmit":        0,
			"tx_queue_0_xdp_xmit_errors": 0,
			"rx_pp_alloc_fast":           0,
			"rx_pp_alloc_slow":           0,
			"rx_pp_alloc_slow_ho":        0,
			"rx_pp_alloc_empty":          0,
			"rx_pp_alloc_refill":         0,
			"rx_pp_alloc_waive":          0,
			"rx_pp_recycle_cached":       0,
			"rx_pp_recycle_cache_full":   0,
			"rx_pp_recycle_ring":         0,
			"rx_pp_recycle_ring_full":    0,
			"rx_pp_recycle_released_ref": 0,
		}, nil
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
	}

	getNewEthtool = func() (ethtoolInterface, error) {
		return mockEthtool, nil
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			statsMap, err := ethtoolObject.Stats(tc.iface)
			if err != nil {
				t.Errorf("%s not implemented in mock", err)
			}
			got := getEthtoolMetrics(tc.iface, statsMap)
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
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.queue.tx_packets", float64(12345), "", expectedTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.queue.rx_packets", float64(67890), "", expectedTags)
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

	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
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

	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
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

	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
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
