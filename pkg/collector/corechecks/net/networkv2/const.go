// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

var (
	ethtoolMetricNames = map[string][]string{
		// Example ethtool -S iface with ena driver:
		// queue_0_tx_cnt: 123665045
		// queue_0_tx_bytes: 34567996008
		// queue_0_tx_queue_stop: 0
		// queue_0_tx_queue_wakeup: 0
		// queue_0_tx_dma_mapping_err: 0
		// queue_0_tx_linearize: 0
		// queue_0_tx_linearize_failed: 0
		// queue_0_tx_napi_comp: 131999879
		// queue_0_tx_tx_poll: 131999896
		// queue_0_tx_doorbells: 117093522
		// queue_0_tx_prepare_ctx_err: 0
		// queue_0_tx_bad_req_id: 0
		// queue_0_tx_llq_buffer_copy: 87883427
		// queue_0_tx_missed_tx: 0
		// queue_0_tx_unmask_interrupt: 131999879
		// queue_0_rx_cnt: 15934470
		// queue_0_rx_bytes: 27955854239
		// queue_0_rx_rx_copybreak_pkt: 8504787
		// queue_0_rx_csum_good: 15923815
		// queue_0_rx_refil_partial: 0
		// queue_0_rx_bad_csum: 0
		// queue_0_rx_page_alloc_fail: 0
		// queue_0_rx_skb_alloc_fail: 0
		// queue_0_rx_dma_mapping_err: 0
		// queue_0_rx_bad_desc_num: 0
		// queue_0_rx_bad_req_id: 0
		// queue_0_rx_empty_rx_ring: 0
		// queue_0_rx_csum_unchecked: 0
		// queue_0_rx_xdp_aborted: 0
		// queue_0_rx_xdp_drop: 0
		// queue_0_rx_xdp_pass: 0
		// queue_0_rx_xdp_tx: 0
		// queue_0_rx_xdp_invalid: 0
		// queue_0_rx_xdp_redirect: 0
		"ena": {
			"rx_bad_csum",
			"rx_bad_desc_num",
			"rx_bad_req_id",
			"rx_bytes",
			"rx_cnt",
			"rx_csum_good",
			"rx_csum_unchecked",
			"rx_dma_mapping_err",
			"rx_empty_rx_ring",
			"rx_page_alloc_fail",
			"rx_refil_partial",
			"rx_rx_copybreak_pkt",
			"rx_skb_alloc_fail",
			"rx_xdp_aborted",
			"rx_xdp_drop",
			"rx_xdp_invalid",
			"rx_xdp_pass",
			"rx_xdp_redirect",
			"rx_xdp_tx",
			"tx_bad_req_id",
			"tx_bytes",
			"tx_cnt",
			"tx_dma_mapping_err",
			"tx_doorbells",
			"tx_linearize",
			"tx_linearize_failed",
			"tx_llq_buffer_copy",
			"tx_missed_tx",
			"tx_napi_comp",
			"tx_prepare_ctx_err",
			"tx_queue_stop",
			"tx_queue_wakeup",
			"tx_tx_poll",
			"tx_unmask_interrupt",
		},
		// Example of output of ethtool -S iface with virtio driver:
		// rx_queue_0_packets: 16591239
		// rx_queue_0_bytes: 51217084980
		// rx_queue_0_drops: 0
		// rx_queue_0_xdp_packets: 0
		// rx_queue_0_xdp_tx: 0
		// rx_queue_0_xdp_redirects: 0
		// rx_queue_0_xdp_drops: 0
		// rx_queue_0_kicks: 408
		// tx_queue_0_packets: 5246609
		// tx_queue_0_bytes: 8455122678
		// tx_queue_0_xdp_tx: 0
		// tx_queue_0_xdp_tx_drops: 0
		// tx_queue_0_kicks: 81
		"virtio_net": {
			"rx_drops",
			"rx_kicks",
			"rx_packets",
			"rx_bytes",
			"rx_xdp_drops",
			"rx_xdp_packets",
			"rx_xdp_redirects",
			"rx_xdp_tx",
			"tx_kicks",
			"tx_packets",
			"tx_bytes",
			"tx_xdp_tx",
			"tx_xdp_tx_drops",
		},
		// Example of output of ethtool -S iface with hv_netvsc driver:
		//  tx_queue_0_packets: 408
		//  tx_queue_0_bytes: 62025
		//  rx_queue_0_packets: 91312
		//  rx_queue_0_bytes: 64734440
		//  rx_queue_0_xdp_drop: 0
		//  tx_queue_1_packets: 0
		//  tx_queue_1_bytes: 0
		//  rx_queue_1_packets: 90945
		//  rx_queue_1_bytes: 66515649
		//  rx_queue_1_xdp_drop: 0
		//  cpu0_rx_packets: 90021
		//  cpu0_rx_bytes: 60954160
		//  cpu0_tx_packets: 2307011
		//  cpu0_tx_bytes: 996614053
		//  cpu0_vf_rx_packets: 762
		//  cpu0_vf_rx_bytes: 1730037
		//  cpu0_vf_tx_packets: 2307011
		//  cpu0_vf_tx_bytes: 996614053
		//  cpu1_rx_packets: 376562
		//  cpu1_rx_bytes: 665669328
		//  cpu1_tx_packets: 3176489
		//  cpu1_tx_bytes: 436967327
		//  cpu1_vf_rx_packets: 266749
		//  cpu1_vf_rx_bytes: 593435159
		//  cpu1_vf_tx_packets: 3176489
		//  cpu1_vf_tx_bytes: 436967327
		"hv_netvsc": {
			// Per queue metrics
			"tx_packets",
			"tx_bytes",
			"rx_packets",
			"rx_bytes",
			"rx_xdp_drop",
			// Per cpu metrics
			"vf_rx_packets",
			"vf_rx_bytes",
			"vf_tx_packets",
			"vf_tx_bytes",
		},
		// ethtool output on an instance with gvnic:
		//      rx_packets: 584088
		//      tx_packets: 17643
		//      rx_bytes: 850689306
		//      tx_bytes: 1420648
		//      rx_dropped: 0
		//      tx_dropped: 0
		//      tx_timeouts: 0
		//      rx_skb_alloc_fail: 0
		//      rx_buf_alloc_fail: 0
		//      rx_desc_err_dropped_pkt: 0
		//      interface_up_cnt: 1
		//      interface_down_cnt: 0
		//      reset_cnt: 0
		//      page_alloc_fail: 0
		//      dma_mapping_error: 0
		//      stats_report_trigger_cnt: 0
		//      rx_posted_desc[0]: 1937
		//      rx_completed_desc[0]: 913
		//      rx_bytes[0]: 558287
		//      rx_dropped_pkt[0]: 0
		//      rx_copybreak_pkt[0]: 538
		//      rx_copied_pkt[0]: 538
		//      rx_queue_drop_cnt[0]: 0
		//      rx_no_buffers_posted[0]: 0
		//      rx_drops_packet_over_mru[0]: 0
		//      rx_drops_invalid_checksum[0]: 0
		//      rx_posted_desc[1]: 263357
		//      rx_completed_desc[1]: 262333
		//      rx_bytes[1]: 382572185
		//      rx_dropped_pkt[1]: 0
		//      rx_copybreak_pkt[1]: 1036
		//      rx_copied_pkt[1]: 172309
		//      rx_queue_drop_cnt[1]: 0
		//      rx_no_buffers_posted[1]: 0
		//      rx_drops_packet_over_mru[1]: 0
		//      rx_drops_invalid_checksum[1]: 0
		//      tx_posted_desc[0]: 2829
		//      tx_completed_desc[0]: 2829
		//      tx_bytes[0]: 221475
		//      tx_wake[0]: 0
		//      tx_stop[0]: 0
		//      tx_event_counter[0]: 2829
		//      tx_dma_mapping_error[0]: 0
		//      tx_posted_desc[1]: 7051
		//      tx_completed_desc[1]: 7051
		//      tx_bytes[1]: 522327
		//      tx_wake[1]: 0
		//      tx_stop[1]: 0
		//      tx_event_counter[1]: 7051
		//      tx_dma_mapping_error[1]: 0
		//      adminq_prod_cnt: 25
		//      adminq_cmd_fail: 0
		//      adminq_timeouts: 0
		//      adminq_describe_device_cnt: 1
		//      adminq_cfg_device_resources_cnt: 1
		//      adminq_register_page_list_cnt: 8
		//      adminq_unregister_page_list_cnt: 0
		//      adminq_create_tx_queue_cnt: 4
		//      adminq_create_rx_queue_cnt: 4
		//      adminq_destroy_tx_queue_cnt: 0
		//      adminq_destroy_rx_queue_cnt: 0
		//      adminq_dcfg_device_resources_cnt: 0
		//      adminq_set_driver_parameter_cnt: 0
		//      adminq_report_stats_cnt: 1
		//      adminq_report_link_speed_cnt: 6
		"gve": {
			"rx_posted_desc",
			"rx_completed_desc",
			"rx_bytes",
			"rx_dropped_pkt",
			"rx_copybreak_pkt",
			"rx_copied_pkt",
			"rx_queue_drop_cnt",
			"rx_no_buffers_posted",
			"rx_drops_packet_over_mru",
			"rx_drops_invalid_checksum",
			"tx_posted_desc",
			"tx_completed_desc",
			"tx_bytes",
			"tx_wake",
			"tx_stop",
			"tx_event_counter",
			"tx_dma_mapping_error",
		},
	}
)

var (
	ethtoolGlobalMetricNames = map[string][]string{
		"ena": {
			"tx_timeout",
			"suspend",
			"resume",
			"wd_expired",
		},
		"hv_netvsc": {
			"tx_scattered",
			"tx_no_memory",
			"tx_no_space",
			"tx_too_big",
			"tx_busy",
			"tx_send_full",
			"rx_comp_busy",
			"rx_no_memory",
			"stop_queue",
			"wake_queue",
		},
		"gve": {
			"tx_timeouts",
			"rx_skb_alloc_fail",
			"rx_buf_alloc_fail",
			"rx_desc_err_dropped_pkt",
			"page_alloc_fail",
			"dma_mapping_error",
		},
	}
)

var (
	protocolsMetricsMapping = map[string]map[string]string{
		"Ip": {
			"InReceives":      "system.net.ip.in_receives",
			"InHdrErrors":     "system.net.ip.in_header_errors",
			"InAddrErrors":    "system.net.ip.in_addr_errors",
			"InUnknownProtos": "system.net.ip.in_unknown_protos",
			"InDiscards":      "system.net.ip.in_discards",
			"InDelivers":      "system.net.ip.in_delivers",
			"OutRequests":     "system.net.ip.out_requests",
			"OutDiscards":     "system.net.ip.out_discards",
			"OutNoRoutes":     "system.net.ip.out_no_routes",
			"ForwDatagrams":   "system.net.ip.forwarded_datagrams",
			"ReasmTimeout":    "system.net.ip.reassembly_timeouts",
			"ReasmReqds":      "system.net.ip.reassembly_requests",
			"ReasmOKs":        "system.net.ip.reassembly_oks",
			"ReasmFails":      "system.net.ip.reassembly_fails",
			"FragOKs":         "system.net.ip.fragmentation_oks",
			"FragFails":       "system.net.ip.fragmentation_fails",
			"FragCreates":     "system.net.ip.fragmentation_creates",
		},
		"IpExt": {
			"InNoRoutes":      "system.net.ip.in_no_routes",
			"InTruncatedPkts": "system.net.ip.in_truncated_pkts",
			"InCsumErrors":    "system.net.ip.in_csum_errors",
			"ReasmOverlaps":   "system.net.ip.reassembly_overlaps",
		},
		"Tcp": {
			"RetransSegs":  "system.net.tcp.retrans_segs",
			"InSegs":       "system.net.tcp.in_segs",
			"OutSegs":      "system.net.tcp.out_segs",
			"ActiveOpens":  "system.net.tcp.active_opens",
			"PassiveOpens": "system.net.tcp.passive_opens",
			"AttemptFails": "system.net.tcp.attempt_fails",
			"EstabResets":  "system.net.tcp.established_resets",
			"InErrs":       "system.net.tcp.in_errors",
			"OutRsts":      "system.net.tcp.out_resets",
			"InCsumErrors": "system.net.tcp.in_csum_errors",
		},
		"TcpExt": {
			"ListenOverflows":      "system.net.tcp.listen_overflows",
			"ListenDrops":          "system.net.tcp.listen_drops",
			"TCPBacklogDrop":       "system.net.tcp.backlog_drops",
			"TCPRetransFail":       "system.net.tcp.failed_retransmits",
			"IPReversePathFilter":  "system.net.ip.reverse_path_filter",
			"PruneCalled":          "system.net.tcp.prune_called",
			"RcvPruned":            "system.net.tcp.prune_rcv_drops",
			"OfoPruned":            "system.net.tcp.prune_ofo_called",
			"PAWSActive":           "system.net.tcp.paws_connection_drops",
			"PAWSEstab":            "system.net.tcp.paws_established_drops",
			"SyncookiesSent":       "system.net.tcp.syn_cookies_sent",
			"SyncookiesRecv":       "system.net.tcp.syn_cookies_recv",
			"SyncookiesFailed":     "system.net.tcp.syn_cookies_failed",
			"TCPAbortOnTimeout":    "system.net.tcp.abort_on_timeout",
			"TCPSynRetrans":        "system.net.tcp.syn_retrans",
			"TCPFromZeroWindowAdv": "system.net.tcp.from_zero_window",
			"TCPToZeroWindowAdv":   "system.net.tcp.to_zero_window",
			"TWRecycled":           "system.net.tcp.tw_reused",
		},
		"Udp": {
			"InDatagrams":  "system.net.udp.in_datagrams",
			"NoPorts":      "system.net.udp.no_ports",
			"InErrors":     "system.net.udp.in_errors",
			"OutDatagrams": "system.net.udp.out_datagrams",
			"RcvbufErrors": "system.net.udp.rcv_buf_errors",
			"SndbufErrors": "system.net.udp.snd_buf_errors",
			"InCsumErrors": "system.net.udp.in_csum_errors",
		},
	}
)

var (
	tcpStateMetricsSuffixMapping = map[string]map[string]string{
		"ss": {
			"ESTAB":      "established",
			"SYN-SENT":   "opening",
			"SYN-RECV":   "opening",
			"FIN-WAIT-1": "closing",
			"FIN-WAIT-2": "closing",
			"TIME-WAIT":  "time_wait",
			"CLOSE-WAIT": "closing",
			"LAST-ACK":   "closing",
			"LISTEN":     "listening",
			"CLOSING":    "closing",
			"UNCONN":     "closing",
			"NONE":       "connections", // sole UDP mapping
		},
		"netstat": {
			"ESTABLISHED": "established",
			"SYN_SENT":    "opening",
			"SYN_RECV":    "opening",
			"FIN_WAIT1":   "closing",
			"FIN_WAIT2":   "closing",
			"TIME_WAIT":   "time_wait",
			"CLOSE":       "closing",
			"CLOSE_WAIT":  "closing",
			"LAST_ACK":    "closing",
			"LISTEN":      "listening",
			"CLOSING":     "closing",
			"NONE":        "connections", // sole UDP mapping
		},
	}

	procfsSubdirectories = []string{"netstat", "snmp"}
)
