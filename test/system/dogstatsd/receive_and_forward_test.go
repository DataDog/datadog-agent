// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func testMetadata(t *testing.T, d *dogstatsdTest) {
	// waiting for metadata payload
	timeOut := time.Tick(10 * time.Second)
	select {
	case <-d.requestReady:
	case <-timeOut:
		require.Fail(t, "Timeout: the backend never receive a metadata requests from dogstatsd")
	}

	requests := d.getRequests()
	require.Len(t, requests, 1)

	metadata := host.Payload{}
	err := json.Unmarshal([]byte(requests[0]), &metadata)
	require.NoError(t, err, "Could not Unmarshal metadata request")

	require.NotNil(t, metadata.Os)
}

func TestReceiveAndForward(t *testing.T) {
	d := setupDogstatsd(t)
	defer d.teardown()
	defer log.Flush()

	testMetadata(t, d)

	d.sendUDP("_sc|test.ServiceCheck|0")

	timeOut := time.Tick(30 * time.Second)
	select {
	case <-d.requestReady:
	case <-timeOut:
		require.Fail(t, "Timeout: the backend never receive a requests from dogstatsd")
	}

	requests := d.getRequests()
	require.Len(t, requests, 1)

	sc := []servicecheck.ServiceCheck{}
	decompressedBody, err := compression.Decompress([]byte(requests[0]))
	require.NoError(t, err, "Could not decompress request body")
	err = json.Unmarshal(decompressedBody, &sc)
	require.NoError(t, err, fmt.Sprintf("Could not Unmarshal request body: %s", decompressedBody))

	require.Len(t, sc, 2)
	assert.Equal(t, sc[0].CheckName, "test.ServiceCheck")
	assert.Equal(t, sc[0].Status, servicecheck.ServiceCheckOK)

	assert.Equal(t, sc[1].CheckName, "datadog.agent.up")
	assert.Equal(t, sc[1].Status, servicecheck.ServiceCheckOK)
}
