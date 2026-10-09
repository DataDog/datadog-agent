// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"encoding/json"
	"fmt"
)

// pingKey is the document key the ping settings arrive under. The snmp handler
// folds them back into its instances until ping is a check of its own.
const pingKey = "ping"

// pingKeyConfig is the value of the "ping" key of the NDM Remote
// Configuration document. The JSON names are the backend contract.
type pingKeyConfig struct {
	InitConfig pingOptions    `json:"init_config"`
	Instances  []pingInstance `json:"instances"`
}

// pingOptions is how a device is pinged, in either init_config or an instance.
type pingOptions struct {
	Count      int              `json:"count"`
	IntervalMS int              `json:"interval_ms"`
	TimeoutMS  int              `json:"timeout_ms"`
	Linux      pingLinuxOptions `json:"linux"`
}

type pingLinuxOptions struct {
	UseRawSocket *bool `json:"use_raw_socket"`
}

// pingInstance is one device of the ping key's instances list. Its presence is
// what enables ping on that device: the schema carries no enabled flag.
type pingInstance struct {
	IPAddress string `json:"ip_address"`
	pingOptions
}

// pingSection is a document's ping settings, indexed by device.
type pingSection struct {
	initConfig    pingOptions
	byIPAddress   map[string]pingOptions
	orderedIPs    []string // for deterministic reporting
	hasInitConfig bool
}

// parsePingSection reads the document's ping key. An absent key yields a nil
// section, which pings nothing.
func parsePingSection(raw json.RawMessage) (*pingSection, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var doc pingKeyConfig
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("the ping key is not an object: %w", err)
	}

	section := &pingSection{
		initConfig:    doc.InitConfig,
		byIPAddress:   make(map[string]pingOptions, len(doc.Instances)),
		hasInitConfig: doc.InitConfig != pingOptions{},
	}
	for _, in := range doc.Instances {
		if in.IPAddress == "" {
			continue
		}
		if _, seen := section.byIPAddress[in.IPAddress]; seen {
			continue
		}
		section.byIPAddress[in.IPAddress] = in.pingOptions
		section.orderedIPs = append(section.orderedIPs, in.IPAddress)
	}
	return section, nil
}

// forDevice returns the ping options of one device, and whether ping is
// enabled on it at all.
func (s *pingSection) forDevice(ipAddress string) (pingOptions, bool) {
	if s == nil {
		return pingOptions{}, false
	}
	options, pinged := s.byIPAddress[ipAddress]
	return options, pinged
}

// shared returns the options every pinged device inherits, or nil when the
// section set none.
func (s *pingSection) shared() *pingOptions {
	if s == nil || !s.hasInitConfig {
		return nil
	}
	return &s.initConfig
}

// unmatched returns the pinged devices that no snmp instance polls, in
// document order. They cannot be scheduled: ping rides on the snmp check.
func (s *pingSection) unmatched(polled map[string]struct{}) []string {
	if s == nil {
		return nil
	}
	var orphans []string
	for _, ip := range s.orderedIPs {
		if _, found := polled[ip]; !found {
			orphans = append(orphans, ip)
		}
	}
	return orphans
}
