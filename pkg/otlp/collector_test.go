// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package otlp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestGetComponents(t *testing.T) {
	_, err := getComponents(&serializer.MockSerializer{})
	// No duplicate component
	require.NoError(t, err)
}
