// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package host

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetOtlpMetaWithOtlp(t *testing.T) {
	config.Datadog.Set(config.OTLPReceiverSection+".protocols.grpc.endpoint", "localhost:9999")
	meta := getOtlpMeta()
	assert.Equal(t, true, meta.Enabled)

	config.Datadog.Set(config.OTLPSection, nil)
	meta = getOtlpMeta()
	assert.Equal(t, false, meta.Enabled)
}
