// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flare

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWindowsFlareProviders guards the one-provider-per-collector split: every
// collector must be registered as its own provider with the expected timeout, so
// a slow collector can no longer starve the others (the flaky missing datadog.reg).
func TestWindowsFlareProviders(t *testing.T) {
	singleCommand := execTimeout + flareProviderTimeoutBuffer
	eventLogConfig := time.Duration(len(eventLogConfigChannels))*execTimeout + flareProviderTimeoutBuffer

	// Expected per-collector timeout; 0 means the default (uncancellable Win32 collectors).
	expected := map[string]time.Duration{
		funcName(getTypeperfData):     singleCommand,
		funcName(getLodctrOutput):     singleCommand,
		funcName(getDatadogRegistry):  singleCommand,
		funcName(getIISData):          singleCommand,
		funcName(getEventLogConfig):   eventLogConfig,
		funcName(getCounterStrings):   0,
		funcName(getWindowsEventLogs): 0,
		funcName(getServiceStatus):    0,
	}

	providers := windowsFlareProviders()
	require.Len(t, providers, len(expected), "one provider per Windows collector")

	seen := map[string]bool{}
	for _, p := range providers {
		name := funcName(p.Callback)
		want, ok := expected[name]
		require.Truef(t, ok, "unexpected Windows flare provider %s", name)
		require.Falsef(t, seen[name], "duplicate Windows flare provider %s", name)
		seen[name] = true
		assert.Equalf(t, want, p.Timeout(nil), "timeout for %s", name)
	}
}

func funcName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}
