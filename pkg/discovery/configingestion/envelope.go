// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configingestion

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

const (
	envelopeService = "configingestion-agent"
	envelopeProject = "config-ingestion-poc"
)

type envelope struct {
	Service string  `json:"service"`
	Project string  `json:"project"`
	Tags    string  `json:"ddtags"`
	Message string  `json:"message"`
	Data    payload `json:"data"`
}

type payload struct {
	HostID       string `json:"host_id"`
	Integration  string `json:"integration"`
	ConfigSource string `json:"config_source"`
	Filename     string `json:"filename"`
	ContentType  string `json:"content_type"`
	Raw          string `json:"raw"`
	Digest       string `json:"digest"`
}

func buildEnvelope(hostID, integration, source, filename, contentType, raw string) envelope {
	sum := sha256.Sum256([]byte(raw))
	return envelope{
		Service: envelopeService,
		Project: envelopeProject,
		// TODO(DSCVR Phase D): make tags config-driven rather than hardcoded.
		Tags: fmt.Sprintf("config-ingestion-poc,source:agent,host:%s", hostID),
		Message: fmt.Sprintf("%s %s config from %s (%s)", integration, source, hostID, filepath.Base(filename)),
		Data: payload{
			HostID:       hostID,
			Integration:  integration,
			ConfigSource: source,
			Filename:     filename,
			ContentType:  contentType,
			Raw:          raw,
			Digest:       hex.EncodeToString(sum[:]),
		},
	}
}
