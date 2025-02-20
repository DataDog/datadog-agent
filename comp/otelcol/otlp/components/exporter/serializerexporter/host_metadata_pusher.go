// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata/payload"
)

type hostMetadataPusher struct {
	forwarder defaultforwarder.Forwarder
}

var _ inframetadata.Pusher = (*hostMetadataPusher)(nil)

func (h *hostMetadataPusher) Push(_ context.Context, hm payload.HostMetadata) error {
	marshaled, err := json.Marshal(hm)
	if err != nil {
		return fmt.Errorf("error marshaling metadata payload: %w", err)
	}

	var buf bytes.Buffer
	g := gzip.NewWriter(&buf)
	if _, err = g.Write(marshaled); err != nil {
		return fmt.Errorf("error compressing metadata payload: %w", err)
	}
	if err = g.Close(); err != nil {
		return fmt.Errorf("error closing gzip writer: %w", err)
	}
	bufBytes := buf.Bytes()
	bytesPayload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&bufBytes})
	return h.forwarder.SubmitHostMetadata(bytesPayload, http.Header{})
}
