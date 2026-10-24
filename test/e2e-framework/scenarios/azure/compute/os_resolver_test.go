// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
)

func TestResolveWindowsURN(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "2025 plain version",
			version: "2025",
			want:    "MicrosoftWindowsServer:WindowsServer:2025-datacenter-azure-edition-core:latest",
		},
		{
			name:    "2025 e2e variant normalizes to the same Marketplace image",
			version: "2025-e2e",
			want:    "MicrosoftWindowsServer:WindowsServer:2025-datacenter-azure-edition-core:latest",
		},
		{
			name:    "2019 e2e variant uses the gensecond URN format",
			version: "2019-e2e",
			want:    "MicrosoftWindowsServer:WindowsServer:2019-datacenter-gensecond:latest",
		},
		{
			name:    "empty version falls back to WindowsServerDefault, e2e suffix normalized",
			version: "",
			want:    "MicrosoftWindowsServer:WindowsServer:" + strings.TrimSuffix(os.WindowsServerDefault.Version, "-e2e") + "-datacenter-azure-edition-core:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urn, err := resolveWindowsURN(azure.Environment{}, os.NewDescriptor(os.WindowsServer, tt.version))
			require.NoError(t, err)
			assert.Equal(t, tt.want, urn)
		})
	}
}
