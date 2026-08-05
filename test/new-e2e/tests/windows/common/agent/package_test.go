// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build e2eunit

package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type packageResolverMock struct {
	mock.Mock
}

func (m *packageResolverMock) getPipelineMSIURL(pipelineID, majorVersion, arch, flavor, nameSuffix string) (string, error) {
	args := m.Called(pipelineID, majorVersion, arch, flavor, nameSuffix)
	return args.String(0), args.Error(1)
}

func (m *packageResolverMock) getProductURL(channelURL, product, version, arch string) (string, error) {
	args := m.Called(channelURL, product, version, arch)
	return args.String(0), args.Error(1)
}

func setupResolverMock(t *testing.T) *packageResolverMock {
	t.Helper()
	m := &packageResolverMock{}
	origPipeline := getPipelineMSIURLFn
	origProduct := getProductURLFn
	getPipelineMSIURLFn = m.getPipelineMSIURL
	getProductURLFn = m.getProductURL
	t.Cleanup(func() {
		getPipelineMSIURLFn = origPipeline
		getProductURLFn = origProduct
	})
	return m
}

// --- NewPackage ---

func TestNewPackage(t *testing.T) {
	tests := []struct {
		name      string
		opts      []PackageOption
		env       map[string]string
		mockSetup func(*packageResolverMock)
		check     func(*testing.T, *Package)
		wantErr   string
	}{
		// --- Release version ---
		{
			name: "release version via options",
			opts: []PackageOption{WithVersion("7.75.0-1")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getProductURL", stableURL, "datadog-agent", "7.75.0-1", defaultArch).
					Return("https://stable.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, stableChannel, p.Channel)
				assert.Equal(t, "https://stable.example.com/agent.msi", p.URL)
			},
		},
		{
			name: "release version via SOURCE_VERSION env",
			opts: []PackageOption{WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_SOURCE_VERSION": "7.75.0-1"},
			mockSetup: func(m *packageResolverMock) {
				m.On("getProductURL", stableURL, "datadog-agent", "7.75.0-1", defaultArch).
					Return("https://stable.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "7.75.0-1", p.Version)
				assert.Equal(t, stableChannel, p.Channel)
				assert.Equal(t, "https://stable.example.com/agent.msi", p.URL)
			},
		},
		{
			name: "SOURCE_VERSION env clears pipeline from prior option",
			opts: []PackageOption{WithPipelineID("old-pipeline"), WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_SOURCE_VERSION": "7.75.0-1"},
			mockSetup: func(m *packageResolverMock) {
				m.On("getProductURL", stableURL, "datadog-agent", "7.75.0-1", defaultArch).
					Return("https://stable.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "7.75.0-1", p.Version)
				assert.Empty(t, p.PipelineID, "PipelineID should be cleared by SOURCE_VERSION")
			},
		},

		// --- RC version ---
		{
			name: "RC version via options",
			opts: []PackageOption{WithVersion("7.76.0-rc.2-1")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getProductURL", betaURL, "datadog-agent", "7.76.0-rc.2-1", defaultArch).
					Return("https://beta.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, betaChannel, p.Channel)
				assert.Equal(t, "https://beta.example.com/agent.msi", p.URL)
			},
		},
		{
			name: "RC version via SOURCE_VERSION env",
			opts: []PackageOption{WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_SOURCE_VERSION": "7.76.0-rc.2-1"},
			mockSetup: func(m *packageResolverMock) {
				m.On("getProductURL", betaURL, "datadog-agent", "7.76.0-rc.2-1", defaultArch).
					Return("https://beta.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, betaChannel, p.Channel)
			},
		},

		// --- Pipeline version ---
		{
			name: "pipeline via options",
			opts: []PackageOption{WithPipelineID("12345")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getPipelineMSIURL", "12345", "7", defaultArch, "", "").
					Return("https://s3.amazonaws.com/pipeline/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "https://s3.amazonaws.com/pipeline/agent.msi", p.URL)
			},
		},
		{
			name: "pipeline via options with flavor",
			opts: []PackageOption{WithPipelineID("12345"), WithFlavor("fips")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getPipelineMSIURL", "12345", "7", defaultArch, "fips", "").
					Return("https://s3.amazonaws.com/pipeline/fips-agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "https://s3.amazonaws.com/pipeline/fips-agent.msi", p.URL)
			},
		},
		{
			name: "pipeline via PIPELINE env",
			opts: []PackageOption{WithArtifactOverrides("TEST")},
			env: map[string]string{
				"TEST_PIPELINE":               "12345",
				"TEST_ASSERT_PACKAGE_VERSION": "7.75.0-devel.git.10.abc1234-1",
			},
			mockSetup: func(m *packageResolverMock) {
				m.On("getPipelineMSIURL", "12345", "7", defaultArch, "", "").
					Return("https://s3.amazonaws.com/pipeline/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "12345", p.PipelineID)
				assert.Equal(t, "7.75.0-devel.git.10.abc1234-1", p.Version, "Version should be set to AssertPackageVersion")
				assert.Equal(t, "https://s3.amazonaws.com/pipeline/agent.msi", p.URL)
			},
		},

		// --- Override trumps option ---
		{
			name: "env pipeline override replaces option-defined release",
			opts: []PackageOption{WithVersion("7.75.0-1"), WithArtifactOverrides("TEST")},
			env: map[string]string{
				"TEST_PIPELINE":               "55555",
				"TEST_ASSERT_PACKAGE_VERSION": "7.75.0-devel.git.10.abc1234-1",
			},
			mockSetup: func(m *packageResolverMock) {
				m.On("getPipelineMSIURL", "55555", "7", defaultArch, "", "").
					Return("https://pipeline.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "55555", p.PipelineID)
				assert.Equal(t, "https://pipeline.example.com/agent.msi", p.URL)
			},
		},
		{
			name: "MSI_URL env bypasses all resolution",
			opts: []PackageOption{WithVersion("7.75.0-1"), WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_MSI_URL": "https://override.example.com/agent.msi"},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "https://override.example.com/agent.msi", p.URL)
			},
		},
		{
			name: "MSI_VERSION wins over SOURCE_VERSION",
			opts: []PackageOption{WithArtifactOverrides("TEST")},
			env: map[string]string{
				"TEST_SOURCE_VERSION": "7.75.0-1",
				"TEST_MSI_VERSION":    "7.80.0-1",
				"TEST_MSI_URL":        "https://override.example.com/agent.msi",
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "7.80.0-1", p.Version, "MSI override should take precedence over SOURCE_VERSION")
				assert.Equal(t, "https://override.example.com/agent.msi", p.URL)
			},
		},
		{
			name: "pipeline takes priority over version",
			opts: []PackageOption{WithPipelineID("12345"), WithVersion("7.75.0-1")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getPipelineMSIURL", "12345", "7", defaultArch, "", "").
					Return("https://pipeline.example.com/agent.msi", nil)
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "https://pipeline.example.com/agent.msi", p.URL)
			},
		},

		// --- Env assertion metadata ---
		{
			name: "assertion vars via env",
			opts: []PackageOption{WithURL("https://example.com/agent.msi"), WithArtifactOverrides("TEST")},
			env: map[string]string{
				"TEST_ASSERT_VERSION":         "7.75.0",
				"TEST_ASSERT_PACKAGE_VERSION": "7.75.0-1",
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "7.75.0", p.AssertAgentVersion)
				assert.Equal(t, "7.75.0-1", p.AssertPackageVersion)
			},
		},

		// --- All MSI overrides ---
		{
			name: "all MSI field overrides via env",
			opts: []PackageOption{WithArtifactOverrides("TEST")},
			env: map[string]string{
				"TEST_MSI_FLAVOR":   "fips",
				"TEST_MSI_PRODUCT":  "datadog-fips-agent",
				"TEST_MSI_ARCH":     "amd64",
				"TEST_MSI_CHANNEL":  "beta",
				"TEST_MSI_VERSION":  "7.80.0-1",
				"TEST_MSI_URL":      "https://override.example.com/agent.msi",
				"TEST_MSI_PIPELINE": "99999",
			},
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "fips", p.Flavor)
				assert.Equal(t, "datadog-fips-agent", p.Product)
				assert.Equal(t, "amd64", p.Arch)
				assert.Equal(t, "beta", p.Channel)
				assert.Equal(t, "7.80.0-1", p.Version)
				assert.Equal(t, "https://override.example.com/agent.msi", p.URL)
				assert.Equal(t, "99999", p.PipelineID)
			},
		},

		// --- Errors ---
		{
			name:    "SOURCE_VERSION and PIPELINE are mutually exclusive",
			opts:    []PackageOption{WithArtifactOverrides("TEST")},
			env:     map[string]string{"TEST_SOURCE_VERSION": "7.75.0-1", "TEST_PIPELINE": "12345"},
			wantErr: "mutually exclusive",
		},
		{
			name: "pipeline S3 lookup error",
			opts: []PackageOption{WithPipelineID("99999")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getPipelineMSIURL", "99999", "7", defaultArch, "", "").
					Return("", errors.New("S3 not found"))
			},
			wantErr: "S3 not found",
		},
		{
			name: "version lookup error",
			opts: []PackageOption{WithVersion("7.99.0-1")},
			mockSetup: func(m *packageResolverMock) {
				m.On("getProductURL", stableURL, "datadog-agent", "7.99.0-1", defaultArch).
					Return("", errors.New("version not found in installers_v2.json"))
			},
			wantErr: "version not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			m := setupResolverMock(t)
			if tc.mockSetup != nil {
				tc.mockSetup(m)
			}
			pkg, err := NewPackage(tc.opts...)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, pkg)
			m.AssertExpectations(t)
		})
	}
}

// --- AgentVersion ---

func TestPackageAgentVersion(t *testing.T) {
	tests := []struct {
		name string
		pkg  Package
		want string
	}{
		{"release from assert field", Package{AssertAgentVersion: "7.75.0"}, "7.75.0"},
		{"RC from assert field", Package{AssertAgentVersion: "7.76.0-rc.2"}, "7.76.0-rc.2"},
		{"devel from assert field", Package{AssertAgentVersion: "7.78.0-devel"}, "7.78.0-devel"},
		{"release fallback from version", Package{Version: "7.75.0-1"}, "7.75.0"},
		{"RC fallback from version", Package{Version: "7.76.0-rc.2-1"}, "7.76.0-rc.2"},
		{"pipeline devel fallback", Package{Version: "7.66.0.git.0.8005fe1.pipeline.65816352-1"}, "7.66.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.pkg.AgentVersion())
		})
	}
}

// --- WithDevEnvOverrides ---

func TestWithDevEnvOverrides(t *testing.T) {
	t.Run("applies overrides when not in CI", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("DEV_ASSERT_VERSION", "7.75.0")
		t.Setenv("DEV_MSI_URL", "https://local.example.com/agent.msi")

		p := &Package{}
		require.NoError(t, WithDevEnvOverrides("DEV")(p))
		assert.Equal(t, "7.75.0", p.AssertAgentVersion)
		assert.Equal(t, "https://local.example.com/agent.msi", p.URL)
	})

	t.Run("skips overrides in CI", func(t *testing.T) {
		t.Setenv("CI", "true")
		t.Setenv("DEV_ASSERT_VERSION", "7.75.0")
		t.Setenv("DEV_MSI_URL", "https://local.example.com/agent.msi")

		p := &Package{Version: "7.70.0-1"}
		require.NoError(t, WithDevEnvOverrides("DEV")(p))
		assert.Empty(t, p.AssertAgentVersion, "env vars should not be applied in CI")
		assert.Empty(t, p.URL, "env vars should not be applied in CI")
		assert.Equal(t, "7.70.0-1", p.Version, "original fields should be preserved in CI")
	})
}
