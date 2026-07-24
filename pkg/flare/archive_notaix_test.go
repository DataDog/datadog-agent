// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !aix

package flare

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

func TestGetSvmonDataNoop(t *testing.T) {
	mock := flarehelpers.NewFlareBuilderMock(t, false)
	err := getSvmonData(context.Background(), mock)
	require.NoError(t, err)

	mock.AssertNoFileExists("svmon.log")
}
