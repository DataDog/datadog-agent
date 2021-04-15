// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"compress/gzip"
	"io"

	"github.com/DataDog/datadog-agent/pkg/trace/export/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/tinylib/msgp/msgp"
)

// EncodePayload encodes the payload as Gzipped msgPack into w.
func EncodePayload(w io.Writer, payload pb.StatsPayload) error {
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return err
	}
	defer func() {
		if err := gz.Close(); err != nil {
			log.Errorf("Error closing gzip stream when writing stats payload: %v", err)
		}
	}()
	return msgp.Encode(gz, &payload)
}
