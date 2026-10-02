// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package output

import (
	"errors"
	"net/http"
	"testing"

	forwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

type inventoryForwarder struct {
	forwarderdef.Forwarder
	calls    int
	payloads transaction.BytesPayloads
	headers  http.Header
	err      error
}

func (f *inventoryForwarder) SubmitMetadata(payloads transaction.BytesPayloads, headers http.Header) error {
	f.calls++
	f.payloads, f.headers = payloads, headers
	return f.err
}

func TestInventoryMetadataUsesDedicatedMetadataForwarder(t *testing.T) {
	want := errors.New("metadata submission failure")
	metrics, metadata := &inventoryForwarder{}, &inventoryForwarder{err: want}
	router := metadataRouter{Forwarder: metrics, metadata: metadata}
	body := []byte(`{"hostname":"synthetic-host","agent_metadata":{"infrastructure_mode":"end_user_device"}}`)
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&body})
	headers := http.Header{"Content-Type": {"application/json"}}
	if err := router.SubmitMetadata(payloads, headers); !errors.Is(err, want) {
		t.Fatalf("changed metadata forwarding result: %v", err)
	}
	if metrics.calls != 0 || metadata.calls != 1 || len(metadata.payloads) != 1 || metadata.payloads[0] != payloads[0] {
		t.Fatal("inventory was rerouted to metric delivery or its body was replaced")
	}
	if metadata.headers.Get("Content-Type") != "application/json" {
		t.Fatal("metadata headers changed")
	}
}
