// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
)

// TestSecrets ensures that secrets placed in environment variables get loaded.
func TestSecrets(t *testing.T) {
	runner := test.Runner{}
	if err := runner.StartAndBuildSecretBackend(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runner.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	c := fmt.Sprintf(`
secret_backend_command: %s
hostanme: ENC[secret1]
`, filepath.Join(runner.BinDir(), test.SecretBackendBinary))
	err := runner.RunAgent([]byte(c))
	if err != nil {
		t.Fatal(err)
	}
}
