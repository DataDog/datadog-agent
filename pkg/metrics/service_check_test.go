// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

func TestMarshalServiceChecks(t *testing.T) {
	serviceChecks := ServiceChecks{{
		CheckName: "test.check",
		Host:      "test.localhost",
		Ts:        1000,
		Status:    ServiceCheckOK,
		Message:   "this is fine",
		Tags:      []string{"tag1", "tag2:yes"},
	}}

	payload, err := serviceChecks.Marshal()
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	newPayload := &agentpayload.ServiceChecksPayload{}
	err = proto.Unmarshal(payload, newPayload)
	assert.Nil(t, err)

	require.Len(t, newPayload.ServiceChecks, 1)
	assert.Equal(t, newPayload.ServiceChecks[0].Name, "test.check")
	assert.Equal(t, newPayload.ServiceChecks[0].Host, "test.localhost")
	assert.Equal(t, newPayload.ServiceChecks[0].Ts, int64(1000))
	assert.Equal(t, newPayload.ServiceChecks[0].Status, int32(ServiceCheckOK))
	assert.Equal(t, newPayload.ServiceChecks[0].Message, "this is fine")
	require.Len(t, newPayload.ServiceChecks[0].Tags, 2)
	assert.Equal(t, newPayload.ServiceChecks[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.ServiceChecks[0].Tags[1], "tag2:yes")
}

func TestMarshalJSONServiceChecks(t *testing.T) {
	serviceChecks := ServiceChecks{{
		CheckName: "my_service.can_connect",
		Host:      "my-hostname",
		Ts:        int64(12345),
		Status:    ServiceCheckOK,
		Message:   "my_service is up",
		Tags:      []string{"tag1", "tag2:yes"},
	}}

	payload, err := serviceChecks.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("[{\"check\":\"my_service.can_connect\",\"host_name\":\"my-hostname\",\"timestamp\":12345,\"status\":0,\"message\":\"my_service is up\",\"tags\":[\"tag1\",\"tag2:yes\"]}]\n"))
}

func TestSplitServiceChecks(t *testing.T) {
	var serviceChecks = ServiceChecks{}
	for i := 0; i < 2; i++ {
		sc := ServiceCheck{
			CheckName: "test.check",
			Host:      "test.localhost",
			Ts:        1000,
			Status:    ServiceCheckOK,
			Message:   "this is fine",
			Tags:      []string{"tag1", "tag2:yes"},
		}
		serviceChecks = append(serviceChecks, &sc)
	}

	newSC, err := serviceChecks.SplitPayload(2)
	require.Nil(t, err)
	require.Len(t, newSC, 2)
	require.Equal(t, 2, len(newSC))

	newSC, err = serviceChecks.SplitPayload(3)
	require.Nil(t, err)
	require.Len(t, newSC, 2)
}
