// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build linux windows darwin

package v5

import (
	"github.com/segmentio/encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metadata/gohai"
	"github.com/stretchr/testify/require"
)

func TestGohaiPayloadMarshalling(t *testing.T) {
	gp := gohai.GetPayload()
	payload := GohaiPayload{MarshalledGohaiPayload{*gp}}
	marshalled, err := json.Marshal(payload)
	require.Nil(t, err)

	var gohaiPayload GohaiPayload
	err = json.Unmarshal(marshalled, &gohaiPayload)
	require.Nil(t, err)
}
