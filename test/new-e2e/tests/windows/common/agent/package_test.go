// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build e2eunit

package agent

import (
	"fmt"
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

// --- NewPackage defaults ---

func TestNewPackageDefaults(t *testing.T) {
	m := setupResolverMock(t)
	_ = m

	pkg, err := NewPackage(WithURL("https://example.com/agent.msi"))
	require.NoError(t, err)
	assert.Equal(t, "datadog-agent", pkg.Product)
	assert.Equal(t, "x86_64", pkg.Arch)
	assert.Equal(t, "https://example.com/agent.msi", pkg.URL)
	m.AssertNotCalled(t, "getPipelineMSIURL", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.AssertNotCalled(t, "getProductURL", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// --- PackageOption functions ---

func TestPackageOptions(t *testing.T) {
	tests := []struct {
		name  string
		opt   PackageOption
		check func(*testing.T, *Package)
	}{
		{
			name: "WithVersion",
			opt:  WithVersion("7.75.0-1"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "7.75.0-1", p.Version)
			},
		},
		{
			name: "WithPipelineID",
			opt:  WithPipelineID("12345"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "12345", p.PipelineID)
			},
		},
		{
			name: "WithChannel",
			opt:  WithChannel("beta"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "beta", p.Channel)
			},
		},
		{
			name: "WithArch",
			opt:  WithArch("amd64"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "amd64", p.Arch)
			},
		},
		{
			name: "WithFlavor",
			opt:  WithFlavor("fips"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "fips", p.Flavor)
			},
		},
		{
			name: "WithProduct",
			opt:  WithProduct("datadog-fips-agent"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "datadog-fips-agent", p.Product)
			},
		},
		{
			name: "WithURL",
			opt:  WithURL("https://example.com/agent.msi"),
			check: func(t *testing.T, p *Package) {
				assert.Equal(t, "https://example.com/agent.msi", p.URL)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Package{}
			err := tc.opt(p)
			require.NoError(t, err)
			tc.check(t, p)
		})
	}
}

// --- Resolve() ---

func TestResolveNoOp(t *testing.T) {
	p := &Package{URL: "https://example.com/agent.msi", PipelineID: "99999"}
	err := p.Resolve()
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/agent.msi", p.URL, "URL should not change when already set")
}

func TestResolveNothingSet(t *testing.T) {
	p := &Package{}
	err := p.Resolve()
	require.NoError(t, err)
	assert.Empty(t, p.URL, "URL should remain empty when nothing is set")
}

func TestResolvePipeline(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getPipelineMSIURL", "12345", "7", "x86_64", "fips", "").
		Return("https://s3.amazonaws.com/pipeline/agent.msi", nil)

	p := &Package{PipelineID: "12345", Arch: "x86_64", Flavor: "fips"}
	require.NoError(t, p.Resolve())
	assert.Equal(t, "https://s3.amazonaws.com/pipeline/agent.msi", p.URL)
	m.AssertExpectations(t)
}

func TestResolvePipelineDefaultArch(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getPipelineMSIURL", "12345", "7", defaultArch, "", "").
		Return("https://s3.amazonaws.com/pipeline/agent.msi", nil)

	p := &Package{PipelineID: "12345"}
	require.NoError(t, p.Resolve())
	assert.Equal(t, "https://s3.amazonaws.com/pipeline/agent.msi", p.URL)
	m.AssertExpectations(t)
}

func TestResolveVersionStable(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getProductURL", stableURL, "datadog-agent", "7.75.0-1", defaultArch).
		Return("https://stable.example.com/agent.msi", nil)

	p := &Package{Version: "7.75.0-1"}
	require.NoError(t, p.Resolve())
	assert.Equal(t, stableChannel, p.Channel)
	assert.Equal(t, "https://stable.example.com/agent.msi", p.URL)
	m.AssertExpectations(t)
}

func TestResolveVersionRC(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getProductURL", betaURL, "datadog-agent", "7.76.0-rc.2-1", defaultArch).
		Return("https://beta.example.com/agent.msi", nil)

	p := &Package{Version: "7.76.0-rc.2-1"}
	require.NoError(t, p.Resolve())
	assert.Equal(t, betaChannel, p.Channel)
	assert.Equal(t, "https://beta.example.com/agent.msi", p.URL)
	m.AssertExpectations(t)
}

func TestResolveVersionPresetChannel(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getProductURL", betaURL, "datadog-agent", "7.75.0-1", defaultArch).
		Return("https://example.com/agent.msi", nil)

	p := &Package{Version: "7.75.0-1", Channel: betaChannel}
	require.NoError(t, p.Resolve())
	assert.Equal(t, betaChannel, p.Channel, "pre-set channel should not be overridden")
	m.AssertExpectations(t)
}

func TestResolveVersionCustomProduct(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getProductURL", stableURL, "datadog-fips-agent", "7.75.0-1", defaultArch).
		Return("https://example.com/fips-agent.msi", nil)

	p := &Package{Version: "7.75.0-1", Product: "datadog-fips-agent"}
	require.NoError(t, p.Resolve())
	m.AssertExpectations(t)
}

func TestResolvePipelineError(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getPipelineMSIURL", "99999", "7", defaultArch, "", "").
		Return("", fmt.Errorf("S3 not found"))

	p := &Package{PipelineID: "99999"}
	err := p.Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3 not found")
	assert.Empty(t, p.URL)
	m.AssertExpectations(t)
}

func TestResolveVersionError(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getProductURL", stableURL, "datadog-agent", "7.99.0-1", defaultArch).
		Return("", fmt.Errorf("version not found in installers_v2.json"))

	p := &Package{Version: "7.99.0-1"}
	err := p.Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version not found")
	assert.Empty(t, p.URL)
	m.AssertExpectations(t)
}

func TestResolvePriorityPipelineBeatsVersion(t *testing.T) {
	m := setupResolverMock(t)
	m.On("getPipelineMSIURL", "12345", "7", defaultArch, "", "").
		Return("https://pipeline.example.com/agent.msi", nil)

	p := &Package{PipelineID: "12345", Version: "7.75.0-1"}
	require.NoError(t, p.Resolve())
	assert.Equal(t, "https://pipeline.example.com/agent.msi", p.URL, "pipeline should take priority over version")
	m.AssertNotCalled(t, "getProductURL", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.AssertExpectations(t)
}

// --- applyEnvOverrides ---

func TestApplyEnvOverridesAssertionVars(t *testing.T) {
	t.Setenv("TEST_ASSERT_VERSION", "7.75.0")
	t.Setenv("TEST_ASSERT_PACKAGE_VERSION", "7.75.0-1")

	p := &Package{}
	err := applyEnvOverrides("TEST", p)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0", p.AssertAgentVersion)
	assert.Equal(t, "7.75.0-1", p.AssertPackageVersion)
}

func TestApplyEnvOverridesSourceVersion(t *testing.T) {
	t.Setenv("TEST_SOURCE_VERSION", "7.75.0-1")

	p := &Package{PipelineID: "old-pipeline"}
	err := applyEnvOverrides("TEST", p)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0-1", p.Version)
	assert.Empty(t, p.PipelineID, "PipelineID should be cleared by _SOURCE_VERSION")
}

func TestApplyEnvOverridesPipeline(t *testing.T) {
	t.Setenv("TEST_PIPELINE", "12345")
	t.Setenv("TEST_ASSERT_PACKAGE_VERSION", "7.75.0-devel.git.10.abc1234-1")

	p := &Package{}
	err := applyEnvOverrides("TEST", p)
	require.NoError(t, err)
	assert.Equal(t, "12345", p.PipelineID)
	assert.Equal(t, "7.75.0-devel.git.10.abc1234-1", p.Version, "Version should be set to AssertPackageVersion")
}

func TestApplyEnvOverridesMutualExclusion(t *testing.T) {
	t.Setenv("TEST_SOURCE_VERSION", "7.75.0-1")
	t.Setenv("TEST_PIPELINE", "12345")

	p := &Package{}
	err := applyEnvOverrides("TEST", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestApplyEnvOverridesMSIOverrides(t *testing.T) {
	t.Setenv("TEST_MSI_FLAVOR", "fips")
	t.Setenv("TEST_MSI_PRODUCT", "datadog-fips-agent")
	t.Setenv("TEST_MSI_ARCH", "amd64")
	t.Setenv("TEST_MSI_CHANNEL", "beta")
	t.Setenv("TEST_MSI_VERSION", "7.80.0-1")
	t.Setenv("TEST_MSI_URL", "https://override.example.com/agent.msi")
	t.Setenv("TEST_MSI_PIPELINE", "99999")

	p := &Package{}
	err := applyEnvOverrides("TEST", p)
	require.NoError(t, err)
	assert.Equal(t, "fips", p.Flavor)
	assert.Equal(t, "datadog-fips-agent", p.Product)
	assert.Equal(t, "amd64", p.Arch)
	assert.Equal(t, "beta", p.Channel)
	assert.Equal(t, "7.80.0-1", p.Version)
	assert.Equal(t, "https://override.example.com/agent.msi", p.URL)
	assert.Equal(t, "99999", p.PipelineID)
}

func TestApplyEnvOverridesMSIOverridesWinOverSourceVersion(t *testing.T) {
	t.Setenv("TEST_SOURCE_VERSION", "7.75.0-1")
	t.Setenv("TEST_MSI_VERSION", "7.80.0-1")
	t.Setenv("TEST_MSI_URL", "https://override.example.com/agent.msi")

	p := &Package{}
	err := applyEnvOverrides("TEST", p)
	require.NoError(t, err)
	assert.Equal(t, "7.80.0-1", p.Version, "MSI override should take precedence over SOURCE_VERSION")
	assert.Equal(t, "https://override.example.com/agent.msi", p.URL)
}

// --- WithArtifactOverrides / WithDevEnvOverrides ---

func TestWithArtifactOverrides(t *testing.T) {
	t.Setenv("MYPREFIX_ASSERT_VERSION", "7.75.0")
	t.Setenv("MYPREFIX_MSI_URL", "https://artifact.example.com/agent.msi")

	p := &Package{}
	opt := WithArtifactOverrides("MYPREFIX")
	err := opt(p)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0", p.AssertAgentVersion)
	assert.Equal(t, "https://artifact.example.com/agent.msi", p.URL)
}

func TestWithDevEnvOverridesNotInCI(t *testing.T) {
	t.Setenv("CI", "")

	t.Setenv("DEV_ASSERT_VERSION", "7.75.0")
	t.Setenv("DEV_MSI_URL", "https://local.example.com/agent.msi")

	p := &Package{}
	opt := WithDevEnvOverrides("DEV")
	err := opt(p)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0", p.AssertAgentVersion)
	assert.Equal(t, "https://local.example.com/agent.msi", p.URL)
}

func TestWithDevEnvOverridesInCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("DEV_ASSERT_VERSION", "7.75.0")
	t.Setenv("DEV_MSI_URL", "https://local.example.com/agent.msi")

	p := &Package{Version: "7.70.0-1"}
	opt := WithDevEnvOverrides("DEV")
	err := opt(p)
	require.NoError(t, err)
	assert.Empty(t, p.AssertAgentVersion, "env vars should not be applied in CI")
	assert.Empty(t, p.URL, "env vars should not be applied in CI")
	assert.Equal(t, "7.70.0-1", p.Version, "original fields should be preserved in CI")
}

// --- AgentVersion() ---

func TestAgentVersionFromAssertField(t *testing.T) {
	p := &Package{AssertAgentVersion: "7.75.0"}
	assert.Equal(t, "7.75.0", p.AgentVersion())
}

func TestAgentVersionFromAssertFieldRC(t *testing.T) {
	p := &Package{AssertAgentVersion: "7.76.0-rc.2"}
	assert.Equal(t, "7.76.0-rc.2", p.AgentVersion())
}

func TestAgentVersionFromAssertFieldDevel(t *testing.T) {
	p := &Package{AssertAgentVersion: "7.78.0-devel"}
	assert.Equal(t, "7.78.0-devel", p.AgentVersion())
}

func TestAgentVersionFallbackStable(t *testing.T) {
	p := &Package{Version: "7.75.0-1"}
	assert.Equal(t, "7.75.0", p.AgentVersion())
}

func TestAgentVersionFallbackRC(t *testing.T) {
	p := &Package{Version: "7.76.0-rc.2-1"}
	assert.Equal(t, "7.76.0-rc.2", p.AgentVersion())
}

func TestAgentVersionFallbackDevelPipeline(t *testing.T) {
	p := &Package{Version: "7.66.0.git.0.8005fe1.pipeline.65816352-1"}
	assert.Equal(t, "7.66.0", p.AgentVersion())
}
