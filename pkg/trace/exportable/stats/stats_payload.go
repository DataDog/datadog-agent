// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"compress/gzip"
	"encoding/json"
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Payload represents the payload to be flushed to the stats endpoint
type Payload struct {
	HostName string   `json:"hostname"`
	Env      string   `json:"env"`
	Stats    []Bucket `json:"stats"`
}

// EncodePayload encodes the payload as Gzipped JSON into w.
func EncodePayload(w io.Writer, payload *Payload) error {
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return err
	}
	defer func() {
		if err := gz.Close(); err != nil {
			log.Errorf("Error closing gzip stream when writing stats payload: %v", err)
		}
	}()
	return json.NewEncoder(gz).Encode(payload)
}
