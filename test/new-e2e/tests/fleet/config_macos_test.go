// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	fleethost "github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/host"
)

// configMacOSSuite covers Fleet configuration-experiment *behavior* on macOS: starting, promoting,
// stopping and rolling back a config experiment, and the on-disk/launchd state that results.
//
// Unlike configSuite (Linux/Windows), every state change here goes through Remote Config via
// fakeintake -- never the installer daemon's local CLI (backend.Backend's runDaemonCommand). That
// CLI path skips the method gate and verifyState reconciliation that only Remote Config exercises;
// see priv_notes/TESTING_MACOS_CONFIG_EXPERIMENT.md's Test 5 discussion. Consequently this suite
// does not embed suite.FleetSuite or use its Backend: FleetSuite's dispatch methods are CLI-only by
// design, and this suite never calls them.
//
// Installation is out of scope: SetupSuite asserts the OCI package repository is already
// registered (the precondition InstallConfigExperiment needs) rather than installing anything
// itself. See the "Known limitation" comment on SetupSuite below for why this currently blocks a
// real run.
type configMacOSSuite struct {
	e2e.BaseSuite[environments.Host]

	Agent *agent.Agent
	Host  *fleethost.Host
}

func newConfigMacOSSuite() e2e.Suite[environments.Host] {
	return &configMacOSSuite{}
}

// TestFleetConfigMacOS runs the macOS Fleet configuration-behavior suite.
//
// macOS E2E hosts are dedicated (mac1.metal/mac2.metal) with a 24-hour AWS billing minimum
// (test/e2e-framework/AGENTS.md), so the CI job for this test must stay manual.
func TestFleetConfigMacOS(t *testing.T) {
	extraConfigMap := runner.ConfigMap{}
	// Pulumi needs to pick a smaller subnet subset on macOS; only settable via the configmap.
	// Without this, RandomSubnets() can pick an AZ (e.g. us-east-1d) that doesn't support
	// mac1.metal/mac2.metal, and Dedicated Host allocation fails with UnsupportedHostConfiguration.
	extraConfigMap.Set("ddinfra:aws/useMacosCompatibleSubnets", "true", false)
	e2e.Run(t, newConfigMacOSSuite(), e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.MacOSDefault), ec2.WithInternetAccess())),
			awshost.WithExtraConfigParams(extraConfigMap),
		),
	))
}

// SetupSuite provisions the suite's helpers and unlocks Remote Config task delivery once.
//
// Known limitation, not fixed here: the real .dmg's postinst (omnibus/package-scripts/agent-dmg/postinst)
// only calls install-stable-jobs today, not the full postInstall hook -- so registerPackageRepository
// never runs on a genuinely .dmg-installed host, and InstallConfigExperiment fails with ENOENT
// (exactly the bug commit a51af836394 fixed for the manually-driven `installer hooks postInstall`
// path this session used, but not for postinst). Until postinst also registers the package
// repository (or the separate installation test set does it), the check below fails fast with a
// clear message instead of leaving every test in this suite to fail deep inside a cryptic ENOENT.
func (s *configMacOSSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.Agent = agent.New(s.T, s.Env())
	s.Host = fleethost.New(s.Env())

	_, err := s.Env().RemoteHost.Execute(
		"test -L /opt/datadog-packages/datadog-agent/stable && test -L /opt/datadog-packages/datadog-agent/experiment")
	require.NoError(s.T(), err, "the OCI package repository is not registered on this host "+
		"(registerPackageRepository has not run). This suite assumes installation -- including "+
		"package-repository registration -- was already done; see the SetupSuite doc comment.")

	// Unlocks UPDATER_TASK delivery for the life of the daemon process. Any oci:// URL with a
	// well-formed sha256 digest satisfies validatePackage; the package is never fetched.
	require.NoError(s.T(), s.fakeintake().RCAddConfig("42", "UPDATER_CATALOG_DD", "catalog-001", "catalog",
		[]byte(`{"packages":[{"package":"datadog-agent","version":"0.0.0",`+
			`"url":"oci://install.datadoghq.com/agent-package@sha256:`+strings.Repeat("0", 64)+`"}]}`)))
}

func (s *configMacOSSuite) fakeintake() *client.Client {
	return s.Env().FakeIntake.Client()
}

// --- Remote Config plumbing -------------------------------------------------------------------
//
// The shapes below mirror pkg/fleet/daemon/remote_config.go's installerConfig/remoteAPIRequest/
// expectedState/experimentTaskParams exactly (those types are unexported, so they can't be
// imported). backend.FileOperation is reused as-is: its JSON tags (file_op/file_path/patch/
// transform/arguments) already match installerConfigFileOperation's wire format byte for byte.

var macOSTaskCounter atomic.Int64

// nextID returns a fresh, monotonically increasing id. UPDATER_TASKs are deduplicated by id for
// the life of the daemon process (remote_config.go's executedRequests), so every push in this
// suite needs one it hasn't used before.
func nextID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, macOSTaskCounter.Add(1))
}

type installerConfigData struct {
	ID             string                  `json:"id"`
	FileOperations []backend.FileOperation `json:"file_operations"`
}

type expectedState struct {
	InstallerVersion string `json:"installer_version"`
	Stable           string `json:"stable"`
	Experiment       string `json:"experiment"`
	StableConfig     string `json:"stable_config"`
	ExperimentConfig string `json:"experiment_config"`
	ClientID         string `json:"client_id"`
}

type encryptedSecret struct {
	Key            string `json:"key"`
	EncryptedValue string `json:"encrypted_value"`
}

type experimentTaskParams struct {
	Version          string            `json:"version"`
	EncryptedSecrets []encryptedSecret `json:"encrypted_secrets"`
}

type updaterTaskData struct {
	ID            string                `json:"id"`
	PackageName   string                `json:"package_name"`
	Method        string                `json:"method"`
	ExpectedState expectedState         `json:"expected_state"`
	Params        *experimentTaskParams `json:"params,omitempty"`
}

// tryReadStatus reads the installer daemon's local API /status, for use inside polling loops
// where a transient failure should be retried, not fail the test outright.
func (s *configMacOSSuite) tryReadStatus() (backend.RemoteConfigState, error) {
	out, err := s.Env().RemoteHost.Execute(
		`sudo curl -sS -H 'Content-Type: application/json' --unix-socket /opt/datadog-agent/run/installer.sock http://installer/status`)
	if err != nil {
		return backend.RemoteConfigState{}, err
	}
	var status backend.RemoteConfigState
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		return backend.RemoteConfigState{}, fmt.Errorf("unmarshal /status output %q: %w", out, err)
	}
	return status, nil
}

// readStatus is tryReadStatus for one-shot reads outside a polling loop.
func (s *configMacOSSuite) readStatus() backend.RemoteConfigState {
	status, err := s.tryReadStatus()
	require.NoError(s.T(), err)
	return status
}

func (s *configMacOSSuite) packageState(status backend.RemoteConfigState) backend.RemoteConfigStatePackage {
	require.Len(s.T(), status.Packages, 1, "expected exactly one package in remote config state")
	return status.Packages[0]
}

// currentExpectedState builds an expected_state that matches the daemon's real state right now, so
// a task built from it passes verifyState. /status's values must be copied verbatim -- including
// the literal "empty" stable_config_version substitution -- never translated; see
// priv_notes/TESTING_MACOS_CONFIG_EXPERIMENT.md's 0.7 for why.
func (s *configMacOSSuite) currentExpectedState() expectedState {
	pkg := s.packageState(s.readStatus())
	return expectedState{
		Stable:           pkg.StableVersion,
		Experiment:       pkg.ExperimentVersion,
		StableConfig:     pkg.StableConfigVersion,
		ExperimentConfig: pkg.ExperimentConfigVersion,
		ClientID:         "disable-client-id-check",
	}
}

func (s *configMacOSSuite) pushInstallerConfig(deploymentID string, fileOps []backend.FileOperation) {
	data, err := json.Marshal(installerConfigData{ID: deploymentID, FileOperations: fileOps})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.fakeintake().RCAddConfig("42", "INSTALLER_CONFIG", "cfg-"+deploymentID, "config", data))
}

func (s *configMacOSSuite) pushTask(method string, expected expectedState, params *experimentTaskParams) {
	taskID := nextID("task-rc")
	data, err := json.Marshal(updaterTaskData{
		ID:            taskID,
		PackageName:   "datadog-agent",
		Method:        method,
		ExpectedState: expected,
		Params:        params,
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.fakeintake().RCAddConfig("42", "UPDATER_TASK", taskID, "task", data))
}

// startConfigExperimentRC pushes the INSTALLER_CONFIG carrying fileOps under deploymentID, then a
// start_experiment_config task referencing it, and waits for the daemon to report the experiment
// live.
func (s *configMacOSSuite) startConfigExperimentRC(deploymentID string, fileOps []backend.FileOperation, secrets map[string]string) {
	expected := s.currentExpectedState()

	var encryptedSecrets []encryptedSecret
	if len(secrets) > 0 {
		pubKeyRaw, err := base64.StdEncoding.DecodeString(s.readStatus().SecretsPubKey)
		require.NoError(s.T(), err)
		require.Len(s.T(), pubKeyRaw, 32, "unexpected secrets public key length")
		var pubKey [32]byte
		copy(pubKey[:], pubKeyRaw)
		for key, value := range secrets {
			enc, err := box.SealAnonymous(nil, []byte(value), &pubKey, rand.Reader)
			require.NoError(s.T(), err)
			encryptedSecrets = append(encryptedSecrets, encryptedSecret{Key: key, EncryptedValue: base64.StdEncoding.EncodeToString(enc)})
		}
	}

	s.pushInstallerConfig(deploymentID, fileOps)
	s.pushTask("start_experiment_config", expected, &experimentTaskParams{Version: deploymentID, EncryptedSecrets: encryptedSecrets})

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, deploymentID, s.packageState(status).ExperimentConfigVersion)
	}, 60*time.Second, 2*time.Second, "experiment_config_version should become %s", deploymentID)
}

func (s *configMacOSSuite) promoteConfigExperimentRC() {
	before := s.currentExpectedState()
	s.pushTask("promote_experiment_config", before, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if !assert.NoError(c, err) {
			return
		}
		pkg := s.packageState(status)
		assert.Equal(c, before.ExperimentConfig, pkg.StableConfigVersion, "the experiment id should now be the stable id")
		assert.Empty(c, pkg.ExperimentConfigVersion, "experiment_config_version should be cleared by promotion")
	}, 60*time.Second, 2*time.Second)
}

func (s *configMacOSSuite) stopConfigExperimentRC() {
	before := s.currentExpectedState()
	s.pushTask("stop_experiment_config", before, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if !assert.NoError(c, err) {
			return
		}
		pkg := s.packageState(status)
		assert.Equal(c, before.StableConfig, pkg.StableConfigVersion, "stable_config_version should not change on stop")
		assert.Empty(c, pkg.ExperimentConfigVersion, "experiment_config_version should be cleared by stop")
	}, 60*time.Second, 2*time.Second)
}

// --- host-state helpers ------------------------------------------------------------------------

// etcExpResting reports whether /opt/datadog-agent/etc-exp is a symlink (the "no experiment
// deployed" state) rather than a real directory.
func (s *configMacOSSuite) etcExpResting() bool {
	out, err := s.Env().RemoteHost.Execute("test -L /opt/datadog-agent/etc-exp && echo symlink || echo directory")
	require.NoError(s.T(), err)
	return strings.TrimSpace(out) == "symlink"
}

func (s *configMacOSSuite) requireResting() {
	require.True(s.T(), s.etcExpResting(), "host must be resting (etc-exp a symlink) before this test starts")
}

// launchdLabelsLoaded returns which of the given labels currently appear in `launchctl list`.
func (s *configMacOSSuite) launchdLabelsLoaded(labels ...string) map[string]bool {
	out, err := s.Env().RemoteHost.Execute("sudo launchctl list | grep datadoghq || true")
	require.NoError(s.T(), err)
	loaded := make(map[string]bool, len(labels))
	for _, label := range labels {
		loaded[label] = strings.Contains(out, label)
	}
	return loaded
}

var swappableJobLabels = []string{"com.datadoghq.agent", "com.datadoghq.sysprobe", "com.datadoghq.data-plane"}

func experimentLabels() []string {
	labels := make([]string, len(swappableJobLabels))
	for i, label := range swappableJobLabels {
		labels[i] = label + "-exp"
	}
	return labels
}

// updaterLogEIOCount counts occurrences of the bootout/bootstrap race's signature in the installer
// daemon's log, so a test can assert it never grows.
func (s *configMacOSSuite) updaterLogEIOCount() int {
	out, err := s.Env().RemoteHost.Execute(
		`sudo grep -cE 'Bootstrap failed|Input/output error' /opt/datadog-agent/logs/updater.log || true`)
	require.NoError(s.T(), err)
	count := 0
	_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count
}

func (s *configMacOSSuite) readFile(path string) string {
	out, err := s.Env().RemoteHost.Execute("sudo cat " + path)
	require.NoError(s.T(), err)
	return out
}

// --- Set 1: basic experiment lifecycle -----------------------------------------------------

func (s *configMacOSSuite) TestConfigStartPromoteMacOS() {
	s.requireResting()
	deploymentID := nextID("cfg-start-promote")

	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
	require.False(s.T(), s.etcExpResting(), "etc-exp should be a real directory during the experiment")

	s.promoteConfigExperimentRC()
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
	require.True(s.T(), s.etcExpResting(), "etc-exp should be a symlink again after promotion")
}

func (s *configMacOSSuite) TestConfigStopRollbackMacOS() {
	s.requireResting()
	deploymentID := nextID("cfg-stop-rollback")

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	previousLogLevel := before["log_level"]

	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])

	s.stopConfigExperimentRC()
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), previousLogLevel, config["log_level"], "stopping must revert to the prior stable config")
	require.True(s.T(), s.etcExpResting())
}

// --- Set 2: repeated experiments ------------------------------------------------------------

func (s *configMacOSSuite) TestMultipleConfigsMacOS() {
	s.requireResting()
	for i := range 3 {
		deploymentID := nextID(fmt.Sprintf("cfg-multi-%d", i))
		s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: fmt.Appendf(nil, `{"tags": ["debug:step-%d"]}`, i)},
		}, nil)
		config, err := s.Agent.Configuration()
		require.NoError(s.T(), err)
		require.Equal(s.T(), []any{fmt.Sprintf("debug:step-%d", i)}, config["tags"])

		s.promoteConfigExperimentRC()
		config, err = s.Agent.Configuration()
		require.NoError(s.T(), err)
		require.Equal(s.T(), []any{fmt.Sprintf("debug:step-%d", i)}, config["tags"])
	}
}

// --- Set 3: jq transform --------------------------------------------------------------------

func (s *configMacOSSuite) TestConfigJQReplaceTagMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-jq-seed"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml",
			Patch: []byte(`{"tags": ["env:staging", "team:fleet", "service:installer"]}`)},
	}, nil)
	s.promoteConfigExperimentRC()

	s.startConfigExperimentRC(nextID("cfg-jq-replace"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationJQ, FilePath: "/datadog.yaml",
			Transform: `.tags|=map(if(.==$old)then($new)else(.)end)`,
			Arguments: []byte(`{"old":"env:staging","new":"env:prod"}`)},
	}, nil)

	assertTags := func() {
		config, err := s.Agent.Configuration()
		require.NoError(s.T(), err)
		raw, ok := config["tags"].([]any)
		require.True(s.T(), ok, "tags should be a list")
		tags := make([]string, len(raw))
		for i, tag := range raw {
			tags[i], ok = tag.(string)
			require.True(s.T(), ok)
		}
		require.ElementsMatch(s.T(), []string{"env:prod", "team:fleet", "service:installer"}, tags)
	}
	assertTags()
	s.promoteConfigExperimentRC()
	assertTags()
}

// --- Set 4: failure and rollback scenarios ---------------------------------------------------

func (s *configMacOSSuite) TestConfigFailureCrashMacOS() {
	s.requireResting()

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)

	s.startConfigExperimentRC(nextID("cfg-crash"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "ENC[invalid_secret]"}`)},
	}, nil)

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), before["log_level"], config["log_level"], "an unresolvable secret placeholder must not reach the resolved config")

	s.stopConfigExperimentRC()
}

// TestConfigRollbackDeploymentIDMacOS pins the expected_state/deployment-ID semantics this session
// found and fixed the doc for: stable_config_version never moves during an experiment, and reverts
// to exactly its prior value on stop, with experiment_config_version cleared.
func (s *configMacOSSuite) TestConfigRollbackDeploymentIDMacOS() {
	s.requireResting()
	initialStableConfigVersion := s.packageState(s.readStatus()).StableConfigVersion

	deploymentID := nextID("cfg-rollback-id")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)

	afterStart := s.packageState(s.readStatus())
	require.Equal(s.T(), deploymentID, afterStart.ExperimentConfigVersion)
	require.Equal(s.T(), initialStableConfigVersion, afterStart.StableConfigVersion, "stable_config_version must not change during an experiment")

	s.stopConfigExperimentRC()

	afterStop := s.packageState(s.readStatus())
	require.Equal(s.T(), initialStableConfigVersion, afterStop.StableConfigVersion,
		"stable_config_version should not change after rollback, got %s (expected %s)", afterStop.StableConfigVersion, initialStableConfigVersion)
	require.Empty(s.T(), afterStop.ExperimentConfigVersion, "experiment_config_version should be empty after rollback")
}

// --- Set 5: file permissions ------------------------------------------------------------------

// TestConfigFilePermissionsMacOS pins macOS's actual ownership model for experiment/stable config
// files: _dd-agent/daemon, not Linux's dd-agent/dd-agent.
func (s *configMacOSSuite) TestConfigFilePermissionsMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-perms"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/conf.d/nginx.yaml",
			Patch: []byte(`{"init_config": {}, "instances": [{"nginx_status_url": "http://localhost:8080/status"}]}`)},
	}, nil)

	assertOwnership := func(path string) {
		perms, err := s.Host.GetFilePermissions(path)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "_dd-agent", perms.Owner, "%s should be owned by _dd-agent", path)
		assert.Equal(s.T(), "daemon", perms.Group, "%s should have group daemon", path)
	}
	assertOwnership("/opt/datadog-agent/etc-exp/datadog.yaml")
	assertOwnership("/opt/datadog-agent/etc-exp/conf.d/nginx.yaml")

	s.promoteConfigExperimentRC()

	assertOwnership("/opt/datadog-agent/etc/datadog.yaml")
	assertOwnership("/opt/datadog-agent/etc/conf.d/nginx.yaml")
}

// --- Set 6: secrets --------------------------------------------------------------------------

func (s *configMacOSSuite) TestConfigWithSecretsMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-secrets"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "SEC[log_level]"}`)},
	}, map[string]string{"log_level": "warn"})

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "warn", config["log_level"])

	s.promoteConfigExperimentRC()
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "warn", config["log_level"])
}

// --- Set 7: integration config loaded during experiment ----------------------------------------

func (s *configMacOSSuite) TestExperimentIntegrationLoadedMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-integration"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/conf.d/nginx.yaml",
			Patch: []byte(`{"init_config": {}, "instances": [{"nginx_status_url": "http://localhost:8080/nginx_status"}]}`)},
	}, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		if !assert.NoError(c, err) {
			return
		}
		_, loaded := status.RunnerStats.Checks["nginx"]
		assert.True(c, loaded, "nginx check should be loaded from the experiment conf.d directory")
	}, 60*time.Second, 5*time.Second)

	s.promoteConfigExperimentRC()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		if !assert.NoError(c, err) {
			return
		}
		_, loaded := status.RunnerStats.Checks["nginx"]
		assert.True(c, loaded, "nginx check should still be loaded after promotion")
	}, 60*time.Second, 5*time.Second)
}

// --- Set 8: system-probe config ----------------------------------------------------------------

func (s *configMacOSSuite) TestSystemProbeConfigMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-sysprobe"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"runtime_security_config": {"enabled": true}}`)},
	}, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe-exp")
		assert.True(c, loaded["com.datadoghq.sysprobe-exp"], "sysprobe experiment job should be loaded")
		status, err := s.Agent.Status()
		assert.NoError(c, err)
		assert.NotEmpty(c, status.AgentMetadata.AgentVersion)
	}, 60*time.Second, 5*time.Second)

	s.promoteConfigExperimentRC()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe")
		assert.True(c, loaded["com.datadoghq.sysprobe"], "sysprobe stable job should be loaded")
		status, err := s.Agent.Status()
		assert.NoError(c, err)
		assert.NotEmpty(c, status.AgentMetadata.AgentVersion)
	}, 60*time.Second, 5*time.Second)
}

// --- Set 9: etc-exp resting-symlink invariant (macOS-specific) -------------------------------

func (s *configMacOSSuite) TestEtcExpRestingSymlinkInvariantMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-etcexp-promote"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	require.False(s.T(), s.etcExpResting(), "etc-exp must become a real directory once an experiment starts")
	s.promoteConfigExperimentRC()
	require.True(s.T(), s.etcExpResting(), "etc-exp must be a symlink again after promote")

	s.startConfigExperimentRC(nextID("cfg-etcexp-stop"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	require.False(s.T(), s.etcExpResting())
	s.stopConfigExperimentRC()
	require.True(s.T(), s.etcExpResting(), "etc-exp must be a symlink again after stop")
}

// --- Set 10: launchd job-set swap (macOS-specific) --------------------------------------------

func (s *configMacOSSuite) TestLaunchdJobSetSwapMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-jobswap"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)

	loaded := s.launchdLabelsLoaded(experimentLabels()...)
	for _, label := range experimentLabels() {
		assert.True(s.T(), loaded[label], "%s should be loaded during the experiment", label)
	}
	out, err := s.Env().RemoteHost.Execute("sudo launchctl print system/com.datadoghq.agent-exp | grep -c KeepAlive || true")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "0", strings.TrimSpace(out), "the experiment job set must not carry KeepAlive")

	s.stopConfigExperimentRC()

	loadedStable := s.launchdLabelsLoaded(swappableJobLabels...)
	for _, label := range swappableJobLabels {
		assert.True(s.T(), loadedStable[label], "%s should be loaded again after stop", label)
	}
	loadedExpAfter := s.launchdLabelsLoaded(experimentLabels()...)
	for _, label := range experimentLabels() {
		assert.False(s.T(), loadedExpAfter[label], "%s should no longer be loaded after stop", label)
	}
}

// --- Set 11: launchd bootout/bootstrap race regression (macOS-specific) -----------------------

// TestLaunchdBootoutBootstrapRaceRegressionMacOS regression-tests the fix in
// pkg/fleet/installer/packages/launchd/launchd.go's Bootout: rapid, back-to-back RC-driven
// start/stop cycles must never leave a job booted out.
func (s *configMacOSSuite) TestLaunchdBootoutBootstrapRaceRegressionMacOS() {
	s.requireResting()
	eioBefore := s.updaterLogEIOCount()

	const cycles = 5
	for i := range cycles {
		deploymentID := nextID(fmt.Sprintf("cfg-race-%d", i))
		s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"tags": ["fleet_exp:race"]}`)},
		}, nil)
		loaded := s.launchdLabelsLoaded(experimentLabels()...)
		for _, label := range experimentLabels() {
			require.True(s.T(), loaded[label], "cycle %d: %s should be loaded", i, label)
		}

		s.stopConfigExperimentRC()
		loadedStable := s.launchdLabelsLoaded(swappableJobLabels...)
		for _, label := range swappableJobLabels {
			require.True(s.T(), loadedStable[label], "cycle %d: %s should be loaded again", i, label)
		}
	}

	require.Equal(s.T(), eioBefore, s.updaterLogEIOCount(),
		"no Bootstrap failed/Input-output error should appear across %d rapid start/stop cycles", cycles)
}

// --- Set 12: config-experiment job definitions match installed (macOS-specific) ----------------

// TestConfigExperimentJobDefinitionsMatchInstalledMacOS regression-tests
// pkg/fleet/installer/packages/datadog_agent_darwin.go's InstallStableJobs: the stable launchd job
// definitions a config experiment writes back on promote must be byte-identical to the ones
// already on disk from install, since both come from the same embedded template.
func (s *configMacOSSuite) TestConfigExperimentJobDefinitionsMatchInstalledMacOS() {
	s.requireResting()

	const plistPath = "/Library/LaunchDaemons/com.datadoghq.agent.plist"
	before := s.readFile(plistPath)

	s.startConfigExperimentRC(nextID("cfg-jobdefs"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.promoteConfigExperimentRC()

	after := s.readFile(plistPath)
	require.Equal(s.T(), before, after, "the stable job definition must not drift across a config-experiment cycle")
}
