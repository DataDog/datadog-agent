// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sk

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var programs = map[string]struct{}{
	"bpf_iter__task_file_socket":          {},
	"bpf_iter__task_file_initial_sockets": {},
	"bpf_iter__task_file_port_bindings":   {},

	"tcp_connect_entry":        {},
	"inet_csk_accept_exit":     {},
	"inet_csk_accept_exit_610": {},
	"tcp_finish_connect_entry": {},
	"tcp_done_entry":           {},
	"tcp_close_entry":          {},
	"tcp_enter_loss_entry":     {},
	"tcp_enter_recovery_entry": {},
	"tcp_send_probe0_entry":    {},
	"tcp_sockops":              {},

	"udp_sendpage_exit":       {},
	"udpv6_sendmsg_exit":      {},
	"udp_sendmsg_exit":        {},
	"skb_consume_udp_entry":   {},
	"udp_destroy_sock_exit":   {},
	"udpv6_destroy_sock_exit": {},
	"udp_post_bind4_cgroup":   {},
	"udp_post_bind6_cgroup":   {},
	"udp_send_skb_entry":      {},
	"udp_v6_send_skb_entry":   {},
}

var fentrySymbols = []string{
	"tcp_connect",
	"tcp_finish_connect",
	"tcp_done",
	"tcp_close",
	"tcp_enter_loss",
	"tcp_enter_recovery",
	"tcp_send_probe0",
	"skb_consume_udp",
	"udp_send_skb",
	"udp_v6_send_skb",
}

var fexitSymbols = []string{
	"inet_csk_accept",
	"udpv6_sendmsg",
	"udp_sendmsg",
	"udp_destroy_sock",
	"udpv6_destroy_sock",
}

func enableProgram(enabled map[string]struct{}, name string) {
	if _, ok := programs[name]; ok {
		enabled[name] = struct{}{}
	}
}

// enabledPrograms returns a map of probes that are enabled per config settings.
func enabledPrograms(c *config.Config) (map[string]struct{}, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]struct{})
	hasSendPage := util.HasTCPSendPage(kv)

	if c.CollectTCPv4Conns || c.CollectTCPv6Conns {
		enableProgram(enabled, "bpf_iter__task_file_socket")
		enableProgram(enabled, "bpf_iter__task_file_initial_sockets")
		enableProgram(enabled, "bpf_iter__task_file_port_bindings")
		enableProgram(enabled, "tcp_connect_entry")
		enableProgram(enabled, "tcp_finish_connect_entry")
		enableProgram(enabled, "tcp_done_entry")
		enableProgram(enabled, "tcp_close_entry")
		enableProgram(enabled, "tcp_enter_loss_entry")
		enableProgram(enabled, "tcp_enter_recovery_entry")
		enableProgram(enabled, "tcp_send_probe0_entry")
		enableProgram(enabled, "tcp_sockops")

		if kv >= kernel.VersionCode(6, 10, 0) {
			enableProgram(enabled, "inet_csk_accept_exit_610")
		} else {
			enableProgram(enabled, "inet_csk_accept_exit")
		}
	}

	if c.CollectUDPv4Conns {
		enableProgram(enabled, "bpf_iter__task_file_socket")
		enableProgram(enabled, "bpf_iter__task_file_initial_sockets")
		enableProgram(enabled, "bpf_iter__task_file_port_bindings")
		enableProgram(enabled, "udp_sendmsg_exit")
		enableProgram(enabled, "skb_consume_udp_entry")
		enableProgram(enabled, "udp_destroy_sock_exit")
		enableProgram(enabled, "udp_post_bind4_cgroup")
		enableProgram(enabled, "udp_send_skb_entry")
	}

	if c.CollectUDPv6Conns {
		enableProgram(enabled, "bpf_iter__task_file_socket")
		enableProgram(enabled, "bpf_iter__task_file_initial_sockets")
		enableProgram(enabled, "bpf_iter__task_file_port_bindings")
		enableProgram(enabled, "udpv6_sendmsg_exit")
		enableProgram(enabled, "skb_consume_udp_entry")
		enableProgram(enabled, "udpv6_destroy_sock_exit")
		enableProgram(enabled, "udp_post_bind6_cgroup")
		enableProgram(enabled, "udp_v6_send_skb_entry")
	}

	if hasSendPage && (c.CollectUDPv4Conns || c.CollectUDPv6Conns) {
		enableProgram(enabled, "udp_sendpage_exit")
	}
	return enabled, nil
}
