// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

//nolint:revive // TODO(PLINT) Fix revive linter
package network

var (
	ETHTOOL_METRIC_NAMES = map[string][]string{
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
	ETHTOOL_GLOBAL_METRIC_NAMES = map[string][]string{
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
