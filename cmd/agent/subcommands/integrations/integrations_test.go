// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/stretchr/testify/assert"
)

func TestMoveConfigurationsFiles(t *testing.T) {
	srcFolder := t.TempDir()
	dstFolder := t.TempDir()
	yamlFiles := []string{"conf.yaml.example", "conf.yaml.default", "metrics.yaml", "auto_conf.yaml"}
	otherFile := "not_moved.txt"
	for _, filename := range append(yamlFiles, otherFile) {
		f, err := os.Create(filepath.Join(srcFolder, filename))
		assert.NoError(t, err)
		assert.NoError(t, f.Close())
	}

	filesCreated, _ := os.ReadDir(srcFolder)
	assert.Equal(t, 5, len(filesCreated))
	for _, file := range filesCreated {
		assert.Contains(t, append(yamlFiles, otherFile), file.Name())
	}

	moveConfigurationFiles(srcFolder, dstFolder)

	filesMoved, _ := os.ReadDir(dstFolder)
	assert.Equal(t, 4, len(filesMoved))
	for _, file := range filesMoved {
		assert.Contains(t, yamlFiles, file.Name())
		// Check that we didn't move the txt file
		assert.NotEqual(t, otherFile, file.Name())
	}
}

func TestMoveConfigurationsFilesProfiles(t *testing.T) {
	srcFolder := t.TempDir()
	dstFolder := t.TempDir()
	os.MkdirAll(filepath.Join(srcFolder, "profiles"), 0755)
	f, err := os.Create(filepath.Join(srcFolder, "profiles", "device.yaml"))
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})

	moveConfigurationFiles(srcFolder, dstFolder)
	_, err = os.Stat(filepath.Join(dstFolder, "profiles", "device.yaml"))
	assert.Nil(t, err)
}

func TestGetVersionFromReqLine(t *testing.T) {
	reqLines := "package1==3.2.1\npackage2==2.3.1"

	version, _, _ := getVersionFromReqLine("package1", reqLines)
	expectedVersion, _ := semver.NewVersion("3.2.1")
	assert.Equal(t, expectedVersion, version)

	version, _, _ = getVersionFromReqLine("package2", reqLines)
	expectedVersion, _ = semver.NewVersion("2.3.1")
	assert.Equal(t, expectedVersion, version)

	version, _, _ = getVersionFromReqLine("package3", reqLines)
	assert.Nil(t, version)

	// Add package2 a second time, should error out
	reqLines += "\npackage2==2.2.0"
	version, _, err := getVersionFromReqLine("package2", reqLines)
	assert.Nil(t, version)
	assert.NotNil(t, err)
}

func TestValidateArgs(t *testing.T) {
	// No args
	args := []string{}
	err := validateArgs(args, false)
	assert.NotNil(t, err)

	// Too many args
	args = []string{"arg1", "arg2"}
	err = validateArgs(args, true)
	assert.NotNil(t, err)

	// Not local => name starts with datadog
	args = []string{"foo"}
	err = validateArgs(args, false)
	assert.NotNil(t, err)
	args = []string{"datadog-foo"}
	err = validateArgs(args, false)
	assert.Nil(t, err)
}

func TestValidateRequirement(t *testing.T) {
	// Case baseVersion < versionReq
	baseVersion, _ := semver.NewVersion("4.1.0")
	versionReq, _ := semver.NewVersion("4.2.0")
	assert.True(t, validateRequirement(baseVersion, "<", versionReq))
	assert.True(t, validateRequirement(baseVersion, "<=", versionReq))
	assert.False(t, validateRequirement(baseVersion, "==", versionReq))
	assert.True(t, validateRequirement(baseVersion, "!=", versionReq))
	assert.False(t, validateRequirement(baseVersion, ">=", versionReq))
	assert.False(t, validateRequirement(baseVersion, ">", versionReq))
	assert.False(t, validateRequirement(baseVersion, "anythingElse", versionReq))

	// Case baseVersion == versionReq
	baseVersion, _ = semver.NewVersion("4.2.0")
	versionReq, _ = semver.NewVersion("4.2.0")
	assert.False(t, validateRequirement(baseVersion, "<", versionReq))
	assert.True(t, validateRequirement(baseVersion, "<=", versionReq))
	assert.True(t, validateRequirement(baseVersion, "==", versionReq))
	assert.False(t, validateRequirement(baseVersion, "!=", versionReq))
	assert.True(t, validateRequirement(baseVersion, ">=", versionReq))
	assert.False(t, validateRequirement(baseVersion, ">", versionReq))
	assert.False(t, validateRequirement(baseVersion, "anythingElse", versionReq))

	// Case baseVersion > versionReq
	baseVersion, _ = semver.NewVersion("4.2.1")
	versionReq, _ = semver.NewVersion("4.2.0")
	assert.False(t, validateRequirement(baseVersion, "<", versionReq))
	assert.False(t, validateRequirement(baseVersion, "<=", versionReq))
	assert.False(t, validateRequirement(baseVersion, "==", versionReq))
	assert.True(t, validateRequirement(baseVersion, "!=", versionReq))
	assert.True(t, validateRequirement(baseVersion, ">=", versionReq))
	assert.True(t, validateRequirement(baseVersion, ">", versionReq))
	assert.False(t, validateRequirement(baseVersion, "anythingElse", versionReq))

}

func TestSemverToPEP440(t *testing.T) {
	assert.Equal(t, semverToPEP440(semver.New("1.3.4")), "1.3.4")
	assert.Equal(t, semverToPEP440(semver.New("1.3.4-rc.1")), "1.3.4rc1")
	assert.Equal(t, semverToPEP440(semver.New("1.3.4-pre.1")), "1.3.4rc1")
	assert.Equal(t, semverToPEP440(semver.New("1.3.4-alpha.1")), "1.3.4a1")
	assert.Equal(t, semverToPEP440(semver.New("1.3.4-beta.1")), "1.3.4b1")
	assert.Equal(t, semverToPEP440(semver.New("1.3.4-beta")), "1.3.4b")
}

func TestPEP440ToSemver(t *testing.T) {
	version, _ := PEP440ToSemver("1.3.4")
	assert.Equal(t, version.String(), "1.3.4")

	version, _ = PEP440ToSemver("12.3.4")
	assert.Equal(t, version.String(), "12.3.4")

	version, _ = PEP440ToSemver("1.32.4")
	assert.Equal(t, version.String(), "1.32.4")

	version, _ = PEP440ToSemver("1.3.42")
	assert.Equal(t, version.String(), "1.3.42")

	version, _ = PEP440ToSemver("1.3.4rc1")
	assert.Equal(t, version.String(), "1.3.4-rc.1")

	version, _ = PEP440ToSemver("1.3.4a1")
	assert.Equal(t, version.String(), "1.3.4-alpha.1")

	version, _ = PEP440ToSemver("1.3.4b1")
	assert.Equal(t, version.String(), "1.3.4-beta.1")

	version, _ = PEP440ToSemver("1.3.4b12")
	assert.Equal(t, version.String(), "1.3.4-beta.12")

	// PEP440 allows this: https://www.python.org/dev/peps/pep-0440/#implicit-pre-release-number
	// We don't ship versions like this, but we support this in case we do in the future.
	version, _ = PEP440ToSemver("1.3.4b")
	assert.Equal(t, version.String(), "1.3.4-beta.0")

	// Other identifiers are passed-through, for resiliency.
	version, _ = PEP440ToSemver("1.3.4dev1")
	assert.Equal(t, version.String(), "1.3.4-dev.1")
}

func TestGetIntegrationName(t *testing.T) {
	assert.Equal(t, getIntegrationName("datadog-checks-base"), "base")
	assert.Equal(t, getIntegrationName("datadog-checks-downloader"), "downloader")
	assert.Equal(t, getIntegrationName("datadog-go-metro"), "go-metro")
	assert.Equal(t, getIntegrationName("datadog-nginx-ingress-controller"), "nginx_ingress_controller")
}

func TestNormalizePackageName(t *testing.T) {
	assert.Equal(t, normalizePackageName("datadog-checks_base"), "datadog-checks-base")
	assert.Equal(t, normalizePackageName("datadog_checks_downloader"), "datadog-checks-downloader")
}

func TestParseWheelPackageNameValidCases(t *testing.T) {
	tests := map[string]struct {
		wheelFileName string
		expectedName  string
	}{
		"name as first line":  {"datadog_my_integration_name_first_line_valid.whl", "datadog-my-integration"},
		"name as second line": {"datadog_my_integration_name_second_line_valid.whl", "datadog-my-integration"},
	}
	for name, test := range tests {
		t.Logf("Running test %s", name)
		name, err := parseWheelPackageName(filepath.Join("..", "..", "..", "testdata", "integrations", test.wheelFileName))
		assert.Equal(t, test.expectedName, name)
		assert.Equal(t, nil, err)
	}
}

func TestParseWheelPackageNameErrorCases(t *testing.T) {
	tests := map[string]struct {
		wheelFileName string
		expectedErr   string
	}{
		"error operning archive file":     {"datadog_my_integration_does_not_exist.whl", "error operning archive file"},
		"package name not found in wheel": {"datadog_my_integration_no_name_invalid.whl", "package name not found in wheel"},
	}
	for name, test := range tests {
		t.Logf("Running test %s", name)
		name, err := parseWheelPackageName(filepath.Join("..", "..", "..", "testdata", "integrations", test.wheelFileName))
		assert.Equal(t, "", name)
		assert.Contains(t, err.Error(), test.expectedErr)
	}
}
