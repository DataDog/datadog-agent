// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDiagnoseCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose"},
		cmdDiagnose,
		func(_ *cliParams, _ core.BundleParams) {})
}

func TestDiagnoseCommandFailsWithoutAPIKey(t *testing.T) {
	t.Setenv("DD_API_KEY", "")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("hostname: test\n"), 0o600))

	root := &cobra.Command{Use: "agent"}
	for _, c := range Commands(&command.GlobalParams{ConfFilePath: dir}) {
		root.AddCommand(c)
	}
	root.SetArgs([]string{"diagnose"})

	err := root.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no API key configured")
	require.Contains(t, err.Error(), "DD_API_KEY")
}

func TestShowMetadataV5Command(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "v5"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataGohaiCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "gohai"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataInventoryAgentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-agent"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowHostGpuCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "host-gpu"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataInventoryHostCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-host"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataInventoryChecksCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-checks"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataHaAgentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "ha-agent"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataPkgSigningCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "package-signing"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataSystemProbeCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "system-probe"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowMetadataSecurityAgentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "security-agent"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowAgentTelemetryCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "agent-telemetry"},
		printPayload,
		func(payload payloadName) {
			require.Equal(t, payloadName("agent-telemetry"), payload)
		})
}

func TestShowFullAgentTelemetryCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "agent-full-telemetry"},
		printAgentFullTelemetry,
		func() {},
	)
}

func TestShowMetadataHostSystemInfoCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "host-system-info"},
		printPayload,
		func(_ core.BundleParams) {})
}

func TestShowHealthIssuesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "health-issues"},
		printHealthPlatformIssues,
		func(_ core.BundleParams) {})
}

// Verifies the bearer token does not leak to the unauthenticated /telemetry endpoint.
func TestPrintAgentFullTelemetryNoBearer(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, "telemetry")
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)

	cfg := configmock.New(t)
	cfg.SetInTest("expvar_port", u.Port())

	ipcComp := ipcmock.New(t)
	err = printAgentFullTelemetry(cfg, ipcComp.GetClient())
	require.NoError(t, err)
	assert.Empty(t, capturedAuth, "Bearer token must not be sent to unauthenticated telemetry endpoint")
}
