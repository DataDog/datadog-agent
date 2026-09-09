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
	"strconv"
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
// The suite installs the Agent itself (see SetupSuite) rather than relying on the provisioner's
// agent install, because it needs the Agent *and* the installer daemon pointed at fakeintake for
// Remote Config, which the provisioner only arranges for its own install.
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
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.MacOSDefault), ec2.WithInternetAccess()), ec2.WithoutAgent()),
			awshost.WithExtraConfigParams(extraConfigMap),
		),
	))
}

// SetupSuite provisions the suite's helpers, installs the Agent, and unlocks Remote Config task
// delivery once.
//
// The provisioner brings up a bare host (ec2.WithoutAgent()) and installation happens here,
// through agent.Agent.Install's macOS path (installMacOSPipeline,
// test/new-e2e/tests/fleet/agent/install.go): the plain .dmg install a customer runs, plus the two
// pieces of configuration the provisioner's agentparams-driven install would otherwise have
// supplied -- remote_updates, without which the installer daemon exits immediately, and the
// remote_configuration block pointing both the Agent and the daemon at this suite's fakeintake.
//
// Set agent.MacOSLocalDMGEnvVar to install a .dmg built from the working tree instead of a pipeline
// build; the assertion below is the reason to bother, since what it checks ships inside the .dmg.
func (s *configMacOSSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.Agent = agent.New(s.T, s.Env())
	s.Host = fleethost.New(s.Env())

	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithRemoteConfig())

	// Asserts the shipped .dmg registered the placeholder OCI package repository, which the whole
	// suite depends on: InstallConfigExperiment reads the package's repository unconditionally, and
	// ConfigAndPackageStates enumerates every registered package to answer /status. Nothing here
	// creates it -- omnibus/package-scripts/agent-dmg/postinst does, via `installer
	// register-package` -- so a failure means that regressed, not that the suite is misconfigured.
	_, err := s.Env().RemoteHost.Execute(
		"test -L /opt/datadog-packages/datadog-agent/stable && test -L /opt/datadog-packages/datadog-agent/experiment")
	require.NoError(s.T(), err, "the .dmg install did not register the Agent in the OCI package "+
		"repository (registerPackageRepository has not run); no configuration experiment can start")

	// fakeintake belongs to the stack, not to the run, so in dev mode (a reused stack) its RC store
	// still holds every config and task the previous run pushed. The daemon replays all of them on
	// its first poll, which both re-deploys whatever that run last experimented with and marks this
	// run's tasks as already executed where the ids collide. Clear the store before pushing
	// anything, and restart the daemon so it polls a clean repository with no executedRequests
	// memory of its own.
	s.resetRemoteConfig()
	_, err = s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/com.datadoghq.installer")
	require.NoError(s.T(), err)
	s.waitForDaemon()

	// A macOS pool host is reused across runs and is not re-imaged, so it can arrive with an
	// experiment left deployed by a run that died mid-test -- and the .dmg install on top does not
	// clear it. Every start_experiment_config the suite then pushes is refused, because the daemon
	// already has a deployment. Roll it back here so the suite starts from the state a fresh host
	// would be in.
	//
	// etc-exp is not the authority for this check: a host has already been seen reporting a stale
	// experiment_config_version while etc-exp was resting, so the config repository's own record
	// is what has to be clear, not just the symlink.
	if !s.etcExpResting() || s.packageState(s.readStatus()).ExperimentConfigVersion != "" {
		s.T().Log("the host arrived with a configuration experiment deployed; rolling it back")
		s.restoreResting()
		require.True(s.T(), s.etcExpResting(), "could not return the host to the resting state")
		require.Empty(s.T(), s.packageState(s.readStatus()).ExperimentConfigVersion,
			"the daemon still reports a deployed configuration experiment")
	}

	// Unlocks UPDATER_TASK delivery for the life of the daemon process. Any oci:// URL with a
	// well-formed sha256 digest satisfies validatePackage; the package is never fetched.
	require.NoError(s.T(), s.fakeintake().RCAddConfig("42", "UPDATER_CATALOG_DD", "catalog-001", "catalog",
		[]byte(`{"packages":[{"package":"datadog-agent","version":"0.0.0",`+
			`"url":"oci://install.datadoghq.com/agent-package@sha256:`+strings.Repeat("0", 64)+`"}]}`)))
}

func (s *configMacOSSuite) fakeintake() *client.Client {
	return s.Env().FakeIntake.Client()
}

// resetRemoteConfig removes every config stored on fakeintake, so the daemon polls a repository
// holding only what this run puts there. Deleting them all -- rather than just this suite's
// products -- is deliberate: anything left behind is by definition from a run that is over.
func (s *configMacOSSuite) resetRemoteConfig() {
	configs, err := s.fakeintake().RCListConfigs()
	require.NoError(s.T(), err)
	for _, config := range configs {
		key := strings.Join([]string{config.OrgID, config.Product, config.ConfigID, config.ConfigName}, "/")
		require.NoError(s.T(), s.fakeintake().RCDeleteConfig(key), "could not delete the leftover config %s", key)
	}
	if len(configs) > 0 {
		s.T().Logf("cleared %d Remote Config entries left on fakeintake by a previous run", len(configs))
	}
}

// --- Remote Config plumbing -------------------------------------------------------------------
//
// The shapes below mirror pkg/fleet/daemon/remote_config.go's installerConfig/remoteAPIRequest/
// expectedState/experimentTaskParams exactly (those types are unexported, so they can't be
// imported). backend.FileOperation is reused as-is: its JSON tags (file_op/file_path/patch/
// transform/arguments) already match installerConfigFileOperation's wire format byte for byte.

var macOSTaskCounter atomic.Int64

// runToken distinguishes this run's Remote Config ids from any other run's.
//
// The counter alone is not enough: it restarts at 1 every run, while fakeintake and its RC store
// survive a reused stack, so "task-rc-1" from a previous run and this run's are the same id --
// which is exactly what UPDATER_TASK deduplication keys on (remote_config.go's executedRequests).
// resetRemoteConfig clears the store as well, and the two together mean a leftover can neither be
// replayed nor be mistaken for something this run pushed.
var runToken = strconv.FormatInt(time.Now().UnixNano(), 36)

// nextID returns a fresh, monotonically increasing id, unique to this run.
func nextID(prefix string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, runToken, macOSTaskCounter.Add(1))
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

// waitForDaemon blocks until the installer daemon answers on its socket again.
//
// launchctl kickstart returns as soon as launchd has been told to restart the job, not when the
// daemon is serving, so the socket is briefly absent afterwards and the next read fails with
// "curl: (7) Failed to connect to installer port 80" rather than with anything about the state
// being asked for.
func (s *configMacOSSuite) waitForDaemon() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		_, err := s.tryReadStatus()
		assert.NoError(c, err)
	}, 60*time.Second, 2*time.Second, "the installer daemon did not come back after being restarted")
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

// pushTaskUntil pushes an UPDATER_TASK and waits for the daemon's own state to satisfy applied,
// re-pushing under a fresh id if it does not.
//
// The re-push is not slow-host paranoia. The suite writes the INSTALLER_CONFIG and the task that
// references it back to back, so a poll landing between the two hands the daemon a task whose
// config it has not fetched yet; it refuses that task ("could not get config: config version ...
// not found in available configs") and then records the id in executedRequests
// (pkg/fleet/daemon/remote_config.go), so it never reconsiders it once the config does arrive. A
// task under a new id is the only way forward, and this race is a normal outcome of the push
// ordering rather than a rare one.
//
// The expected_state is rebuilt per attempt, since a refused task leaves the state untouched but a
// partially applied one does not.
func (s *configMacOSSuite) pushTaskUntil(method string, params *experimentTaskParams,
	applied func(backend.RemoteConfigStatePackage) bool, description string) {
	const (
		attempts = 3
		perTry   = 25 * time.Second
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		s.pushTask(method, s.currentExpectedState(), params)
		if s.waitForPackageState(applied, perTry) {
			return
		}
		s.T().Logf("%s: the daemon did not apply the task within %s (attempt %d/%d); pushing it again under a new id",
			description, perTry, attempt, attempts)
	}
	require.FailNow(s.T(), description+": the daemon never applied the task")
}

// waitForPackageState polls the daemon's /status until applied accepts the package state, and
// reports whether it did. It is deliberately non-fatal: pushTaskUntil treats a timeout as a reason
// to push again, not as a test failure.
func (s *configMacOSSuite) waitForPackageState(applied func(backend.RemoteConfigStatePackage) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		// /status is read straight through rather than via packageState, whose require.Len would turn
		// a transient read into a failure in the middle of a poll loop.
		if status, err := s.tryReadStatus(); err == nil && len(status.Packages) == 1 && applied(status.Packages[0]) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Second)
	}
}

// startConfigExperimentRC pushes the INSTALLER_CONFIG carrying fileOps under deploymentID, then a
// start_experiment_config task referencing it, and waits for the daemon to report the experiment
// live.
func (s *configMacOSSuite) startConfigExperimentRC(deploymentID string, fileOps []backend.FileOperation, secrets map[string]string) {
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
	s.pushTaskUntil("start_experiment_config",
		&experimentTaskParams{Version: deploymentID, EncryptedSecrets: encryptedSecrets},
		func(pkg backend.RemoteConfigStatePackage) bool { return pkg.ExperimentConfigVersion == deploymentID },
		fmt.Sprintf("start_experiment_config %s", deploymentID))
}

func (s *configMacOSSuite) promoteConfigExperimentRC() {
	before := s.currentExpectedState()
	s.pushTaskUntil("promote_experiment_config", nil, func(pkg backend.RemoteConfigStatePackage) bool {
		// The promoted experiment becomes the stable configuration, and nothing is left deployed.
		return pkg.StableConfigVersion == before.ExperimentConfig && pkg.ExperimentConfigVersion == ""
	}, fmt.Sprintf("promote_experiment_config %s", before.ExperimentConfig))
}

func (s *configMacOSSuite) stopConfigExperimentRC() {
	before := s.currentExpectedState()
	s.pushTaskUntil("stop_experiment_config", nil, func(pkg backend.RemoteConfigStatePackage) bool {
		// A stop rolls the experiment back, so the stable configuration is the one it already was.
		return pkg.StableConfigVersion == before.StableConfig && pkg.ExperimentConfigVersion == ""
	}, fmt.Sprintf("stop_experiment_config %s", before.ExperimentConfig))
}

// --- host-state helpers ------------------------------------------------------------------------

// etcExpResting reports whether no configuration experiment is deployed, matching
// restingLink.IsResting (pkg/fleet/installer/config/resting_link.go): a symlink to the stable
// configuration means nothing is deployed, a real directory means something is, and an absent
// path also counts as resting -- the install does not create etc-exp, only Rest does, so a host
// that has never run an experiment has nothing there at all.
//
// -L is tested before -e so that a dangling symlink reads as a symlink rather than as absent.
func (s *configMacOSSuite) etcExpResting() bool {
	out, err := s.Env().RemoteHost.Execute(
		"if [ -L /opt/datadog-agent/etc-exp ]; then echo symlink; " +
			"elif [ -e /opt/datadog-agent/etc-exp ]; then echo directory; else echo absent; fi")
	require.NoError(s.T(), err)
	state := strings.TrimSpace(out)
	return state == "symlink" || state == "absent"
}

func (s *configMacOSSuite) requireResting() {
	require.True(s.T(), s.etcExpResting(), "host must be resting (etc-exp a symlink or absent) before this test starts")
}

// restoreResting rolls back a deployed configuration experiment, best effort.
//
// The rollback goes through the installer rather than by deleting etc-exp by hand, so the config
// repository's state and the launchd jobs follow the configuration back. It has to be
// remove-config-experiment and not the daemon's stop-config-experiment: the .dmg ships the
// standalone installer (pkg/fleet/installer/commands), which on macOS carries the config-experiment
// commands but none of the daemon subcommands (cmd/installer/subcommands/daemon), so
// stop-config-experiment does not exist on the host at all.
func (s *configMacOSSuite) restoreResting() {
	out, err := s.Env().RemoteHost.Execute(
		"sudo /opt/datadog-agent/embedded/bin/installer remove-config-experiment datadog-agent")
	if err != nil {
		s.T().Logf("could not roll back the deployed configuration experiment: %v (%s)", err, out)
		return
	}
	if !s.etcExpResting() {
		s.T().Log("a configuration experiment is still deployed after remove-config-experiment")
	}
}

// dumpDiagnostics logs what a failed configuration-experiment test needs to be explained: the
// state the daemon reports, and what it logged while reaching it. Nothing in a Remote Config
// failure is visible from the test side -- a task the daemon refuses is indistinguishable from one
// it never received -- so the daemon's own log is the only place the reason exists.
func (s *configMacOSSuite) dumpDiagnostics() {
	if status, err := s.tryReadStatus(); err == nil {
		s.T().Logf("installer daemon state: %+v", status)
	} else {
		s.T().Logf("could not read the installer daemon state: %v", err)
	}
	// The uptane store logs one line per key on every poll, which is thousands of lines saying
	// nothing about why a task was refused; drop them or they crowd out the reason entirely.
	if out, err := s.Env().RemoteHost.Execute(
		"sudo grep -v transactional_store.go /opt/datadog-agent/logs/updater.log | tail -n 100"); err == nil {
		s.T().Logf("last 100 lines of updater.log:\n%s", out)
	} else {
		s.T().Logf("could not read the installer daemon log: %v", err)
	}
}

// AfterTest returns the host to the resting state, so that one failure costs one test instead of
// the whole suite: every test starts with requireResting, and a test that fails between start and
// stop leaves the experiment deployed -- which then fails that precondition in every test
// scheduled after it.
func (s *configMacOSSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)
	if s.T().Failed() {
		s.dumpDiagnostics()
	}
	if s.etcExpResting() {
		return
	}
	s.T().Logf("%s left a configuration experiment deployed; rolling it back to restore the resting state", testName)
	s.restoreResting()
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

// TestConfigFailureCrashMacOS pins that an experiment carrying an unresolvable secret placeholder
// cannot make the bad value stick.
//
// The resolved config is read after the rollback, not during the experiment: ENC[...] is resolved
// by the Agent at startup through the secret backend, so agent-exp exits before it ever binds the
// IPC port, and nothing answers https://localhost:5001/agent/config while the experiment is
// deployed. Reading it there fails with "connection refused" rather than with the wrong log_level,
// which tests the harness and not the product. What the deployment did is checked on disk instead.
func (s *configMacOSSuite) TestConfigFailureCrashMacOS() {
	s.requireResting()

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)

	s.startConfigExperimentRC(nextID("cfg-crash"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "ENC[invalid_secret]"}`)},
	}, nil)

	// The experiment is written out even though the Agent it configures cannot come up.
	require.False(s.T(), s.etcExpResting(), "etc-exp should be a real directory during the experiment")
	require.Contains(s.T(), s.readFile("/opt/datadog-agent/etc-exp/datadog.yaml"), "ENC[invalid_secret]")

	s.stopConfigExperimentRC()

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), before["log_level"], config["log_level"],
		"an unresolvable secret placeholder must not survive the rollback")
	require.True(s.T(), s.etcExpResting())
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
