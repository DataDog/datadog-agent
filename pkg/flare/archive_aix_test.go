// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package flare

import (
	"context"
	"testing"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/stretchr/testify/assert"
)

func TestGetErrptData(t *testing.T) {
	mock := flarehelpers.NewFlareBuilderMock(t, false)
	assert.NoError(t, getErrptData(context.Background(), mock))

	// The section headers are always present; the errpt output itself depends
	// on the error log content and on running the test as root.
	mock.AssertFileContentMatch(
		`(?s)=+ Process deaths by signal.*=+ SRC subsystem lifecycle.*`,
		"errpt.log")
}
