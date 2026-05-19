// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// envelope mirrors the EvP intake envelope consumed by the demoalpha-worker.
// See DataDog/experimental#9989 (phase3/sender/send-demoalpha.sh) for the
// canonical wire shape.
type envelope struct {
	Service string  `json:"service"`
	Project string  `json:"project"`
	Tags    string  `json:"ddtags"`
	Message string  `json:"message"`
	Data    payload `json:"data"`
}

// payload carries the per-config fields the worker reads from data.*.
// All five fields host_id / integration / config_source / content_type /
// raw must be non-empty for the worker to write into config_facts_v1.
type payload struct {
	HostID       string `json:"host_id"`
	Integration  string `json:"integration"`
	ConfigSource string `json:"config_source"`
	Filename     string `json:"filename"`
	ContentType  string `json:"content_type"`
	Raw          string `json:"raw"`
}

// buildEnvelope returns the serialized envelope and a sha256 hex digest of
// the raw payload.
func buildEnvelope(hostID, integration, source, filename, contentType string, raw []byte) ([]byte, string, error) {
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	e := envelope{
		Service: "demoalpha-worker",
		Project: "config-ingestion-poc",
		Tags:    fmt.Sprintf("config-ingestion-poc,source:agent,host:%s", hostID),
		Message: fmt.Sprintf("%s %s config from %s (%s)", integration, source, hostID, filename),
		Data: payload{
			HostID:       hostID,
			Integration:  integration,
			ConfigSource: source,
			Filename:     filename,
			ContentType:  contentType,
			Raw:          string(raw),
		},
	}
	b, err := json.Marshal(e)
	return b, hash, err
}
