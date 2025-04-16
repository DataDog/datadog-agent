// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
)

type chainConntracker struct {
	ctks []netlink.Conntracker
}

func chainConntrackers(ctks ...netlink.Conntracker) netlink.Conntracker {
	if len(ctks) == 1 {
		return ctks[0]
	}

	// filter out no-ops
	filtered := ctks[:0]
	for _, ctk := range ctks {
		if ctk != nil && ctk.GetType() != "" {
			filtered = append(filtered, ctk)
		}
	}

	if len(filtered) == 0 {
		return netlink.NewNoOpConntracker()
	}

	if len(filtered) == 1 {
		return filtered[0]
	}

	return &chainConntracker{
		ctks: filtered,
	}
}

// Describe returns all descriptions of the collector
func (ct *chainConntracker) Describe(descs chan<- *prometheus.Desc) {
	for _, ctk := range ct.ctks {
		ctk.Describe(descs)
	}
}

// Collect returns the current state of all metrics of the collector
func (ct *chainConntracker) Collect(metrics chan<- prometheus.Metric) {
	for _, ctk := range ct.ctks {
		ctk.Collect(metrics)
	}
}

func (ct *chainConntracker) GetTranslationForConn(c *network.ConnectionTuple) *network.IPTranslation {
	for _, ctk := range ct.ctks {
		if trans := ctk.GetTranslationForConn(c); trans != nil {
			return trans
		}
	}

	return nil
}

// GetType returns a string describing whether the conntracker is "ebpf" or "netlink"
func (ct *chainConntracker) GetType() string {
	return "chain"
}

func (ct *chainConntracker) DeleteTranslation(c *network.ConnectionTuple) {
	for _, ctk := range ct.ctks {
		ctk.DeleteTranslation(c)
	}
}

func (ct *chainConntracker) DumpCachedTable(ctx context.Context) (map[uint32][]netlink.DebugConntrackEntry, error) {
	res := map[uint32][]netlink.DebugConntrackEntry{}
	for _, ctk := range ct.ctks {
		var m map[uint32][]netlink.DebugConntrackEntry
		var err error
		if m, err = ctk.DumpCachedTable(ctx); err != nil {
			return res, err
		}

		for k, v := range m {
			res[k] = append(res[k], v...)
		}
	}

	return res, nil
}

func (ct *chainConntracker) Close() {
	for _, ctk := range ct.ctks {
		ctk.Close()
	}
}
