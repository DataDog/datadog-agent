// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sharedlibrary

import (
	//"fmt"
	"runtime"
	"testing"
	//"time"
	//"unsafe"

	"github.com/stretchr/testify/assert"
	//"github.com/stretchr/testify/require"

	//"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	//diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	//"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	//checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

/*
#include "shared_library.h"

static char *mock_run_symbol(char *check_id, char *init_config, char *instance_config, const aggregator_t *callbacks) {
	printf("mock run symbol");
	return NULL;
}

handles_t get_mock_lib_handles(void) {
	// only the symbol is required to run the check, so the library handle can be set to NULL
	handles_t lib_handles = { NULL, mock_run_symbol };
	return lib_handles;
}
*/
import "C"

func testRunCheck(t *testing.T) {
	check, err := NewSharedLibraryFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	err = check.runCheckImpl(false)
	assert.Nil(t, err)
}

func testRunCheckWithNullSymbol(t *testing.T) {
	check, err := NewSharedLibraryFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	// set the symbol handle to NULL
	check.libHandles.run = nil

	err = check.runCheckImpl(false)
	assert.Error(t, err, "pointer to shared library 'Run' symbol is NULL")
}

// NewSharedLibraryFakeCheck creates a fake SharedLibraryCheck
func NewSharedLibraryFakeCheck(senderManager sender.SenderManager) (*SharedLibraryCheck, error) {
	c, err := NewSharedLibraryCheck(senderManager, "fake_check", C.get_mock_lib_handles())

	// Remove check finalizer that may trigger race condition while testing
	if err == nil {
		runtime.SetFinalizer(c, nil)
	}

	return c, err
}
