// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package snmp turns the "snmp" key of the NDM Remote Configuration document
// into snmp check configs.
package snmp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	snmpcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Key is the document key this handler owns.
const Key = "snmp"

// configSource is the Source every config this handler produces carries.
const configSource = names.NDMRemoteConfig + ":" + Key

// Handler turns the "snmp" key of an Agent's NDM document into one snmp check
// config per device.
type Handler struct {
	creds *credentialStore
	log   log.Component
}

// NewHandler builds the snmp document-key handler.
func NewHandler(cfg model.Reader, logComp log.Component) *Handler {
	return &Handler{creds: newCredentialStore(cfg), log: logComp}
}

// Key returns the document key this handler owns.
func (h *Handler) Key() string { return Key }

// Render turns a path's snmp key into one check config per device, folding in the
// ping sibling key. An instance with no usable credential is skipped, not scheduled.
func (h *Handler) Render(path string, raw json.RawMessage, siblings map[string]json.RawMessage) ([]integration.Config, error) {
	var doc keyConfig
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("the snmp key is not an object: %w", err)
	}

	ping, err := parsePingSection(siblings[pingKey])
	if err != nil {
		h.log.Errorf("ndm: ignoring the ping settings of config %s: %v", path, err)
	}

	initConfigData, err := renderInitConfig(doc.InitConfig, ping.shared())
	if err != nil {
		return nil, err
	}

	creds, err := h.creds.load()
	if err != nil {
		h.log.Warnf("ndm: some snmp credential files of config %s did not load: %v", path, err)
	}

	configs := make([]integration.Config, 0, len(doc.Instances))
	polled := make(map[string]struct{}, len(doc.Instances))
	var skipped []string

	for _, instance := range doc.Instances {
		if instance.IPAddress != "" {
			polled[instance.IPAddress] = struct{}{}
		}

		cred, reason := resolve(instance, creds)
		if reason != "" {
			skipped = append(skipped, reason)
			h.log.Warnf("ndm: skipping an snmp instance of config %s: %s", path, reason)
			continue
		}

		devicePing, pinged := ping.forDevice(instance.IPAddress)
		var pingOverrides *pingOptions
		if pinged {
			pingOverrides = &devicePing
		}

		instanceData, err := renderInstance(instance, cred, pingOverrides)
		if err != nil {
			skipped = append(skipped, err.Error())
			h.log.Warnf("ndm: skipping an snmp instance of config %s: %v", path, err)
			continue
		}

		configs = append(configs, integration.Config{
			Name:       snmpcheck.CheckName,
			Source:     configSource,
			InitConfig: initConfigData,
			Instances:  []integration.Data{instanceData},
		})
	}

	if orphans := ping.unmatched(polled); len(orphans) > 0 {
		h.log.Errorf("ndm: config %s pings %d device(s) no snmp instance polls, so they cannot be scheduled: %s",
			path, len(orphans), strings.Join(orphans, ", "))
	}

	if len(skipped) > 0 {
		return configs, errors.New(strings.Join(skipped, "; "))
	}
	return configs, nil
}

// resolve joins an instance with its credential, or returns why it cannot be
// scheduled. The reason never names a credential value.
func resolve(instance documentInstance, creds map[string]credential) (credential, string) {
	if instance.IPAddress == "" {
		return credential{}, fmt.Sprintf("an instance referencing credential %q has no ip_address", instance.CredID)
	}
	cred, found := creds[instance.CredID]
	if !found {
		return credential{}, fmt.Sprintf("%s references credential %q, which is not available on this Agent", instance.IPAddress, instance.CredID)
	}
	if err := validate(cred); err != nil {
		return credential{}, fmt.Sprintf("%s cannot be scheduled: %s", instance.IPAddress, err.Error())
	}
	return cred, ""
}
