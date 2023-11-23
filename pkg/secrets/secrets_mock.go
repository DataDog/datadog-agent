// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets && test

package secrets

import (
	"testing"
)

// InjectSecrets inject a value for an handle into the secrets cache. This allows to use secrets in tests.
func InjectSecrets(t *testing.T, handle string, value string) {
	secretCache[handle] = value
	t.Cleanup(func() {
		delete(secretCache, handle)
	})
}
