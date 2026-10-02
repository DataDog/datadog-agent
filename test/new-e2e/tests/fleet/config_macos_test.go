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
// CLI path skips the verifyState reconciliation that only Remote Config exercises;
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

	// stableConfigBaseline is stableConfigFingerprint of the configuration SetupSuite archived.
	stableConfigBaseline string
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
	// postinst datadog-agent dmg` -- so a failure means that regressed, not that the suite is misconfigured.
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

	s.saveStableConfigBaseline()
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

// clearPendingUpdaterTasks removes every UPDATER_TASK still sitting on fakeintake, so a daemon
// restart does not see (and redeliver) a task issued before the simulated crash.
//
// fakeintake keeps a pushed UPDATER_TASK assigned until it is explicitly deleted, mirroring a real
// Remote Config backend's targets. In production that deletion happens once the backend observes the
// client's applied status; this test's RC poll interval is seconds, so a real crash-and-restart cycle
// -- which plays out over minutes, long after the backend would have settled the task -- is
// compressed here into a gap too short for that to happen naturally. Left unhandled, a task this
// test already applied (start_experiment_config) gets redelivered right after restart and
// verifyState lets it re-apply, because a successful revert leaves the daemon's state exactly
// matching that task's pre-state. That is a test-timing artifact of the fast local loop, not a
// product bug: a real backend would not still be offering this task minutes after it first applied.
func (s *configMacOSSuite) clearPendingUpdaterTasks() {
	configs, err := s.fakeintake().RCListConfigs()
	require.NoError(s.T(), err)
	for _, config := range configs {
		if config.Product != "UPDATER_TASK" {
			continue
		}
		key := strings.Join([]string{config.OrgID, config.Product, config.ConfigID, config.ConfigName}, "/")
		require.NoError(s.T(), s.fakeintake().RCDeleteConfig(key), "could not delete the pending task %s", key)
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

// pushTask pushes an UPDATER_TASK under a fresh id and returns that id.
func (s *configMacOSSuite) pushTask(method string, expected expectedState, params *experimentTaskParams) string {
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
	return taskID
}

// pushTaskUntilExecuted pushes an UPDATER_TASK and returns it as /status reports it once the daemon
// has finished executing it, successfully or not.
//
// It re-pushes under a fresh id for the same reason pushTaskUntil does: a task that arrives before
// its INSTALLER_CONFIG fails with "not found in available configs", which is the push race and not
// an outcome of the task itself.
func (s *configMacOSSuite) pushTaskUntilExecuted(method string, params *experimentTaskParams, description string) backend.RemoteConfigStateTask {
	const (
		attempts = 3
		perTry   = 25 * time.Second
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		taskID := s.pushTask(method, s.currentExpectedState(), params)
		var task *backend.RemoteConfigStateTask
		s.waitForPackageState(func(pkg backend.RemoteConfigStatePackage) bool {
			if pkg.Task == nil || pkg.Task.ID != taskID {
				return false
			}
			if pkg.Task.State != backend.TaskStateDone && pkg.Task.State != backend.TaskStateError {
				return false
			}
			task = pkg.Task
			return true
		}, perTry)
		switch {
		case task == nil:
			s.T().Logf("%s: the daemon did not execute the task within %s (attempt %d/%d)", description, perTry, attempt, attempts)
		case task.Error != nil && strings.Contains(task.Error.Message, "not found in available configs"):
			s.T().Logf("%s: the task arrived before its config (attempt %d/%d); pushing it again under a new id", description, attempt, attempts)
		default:
			return *task
		}
	}
	require.FailNow(s.T(), description+": the daemon never finished executing the task")
	return backend.RemoteConfigStateTask{}
}

// pushTaskExpectingFailure is pushTaskUntilExecuted for a task the daemon is expected to fail.
func (s *configMacOSSuite) pushTaskExpectingFailure(method string, params *experimentTaskParams, description string) backend.RemoteConfigStateTask {
	task := s.pushTaskUntilExecuted(method, params, description)
	require.Equal(s.T(), backend.TaskStateError, task.State, "%s: the task succeeded, but it was expected to fail", description)
	return task
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
		"start_experiment_config "+deploymentID)
}

func (s *configMacOSSuite) promoteConfigExperimentRC() {
	before := s.currentExpectedState()
	s.pushTaskUntil("promote_experiment_config", nil, func(pkg backend.RemoteConfigStatePackage) bool {
		// The promoted experiment becomes the stable configuration, and nothing is left deployed.
		return pkg.StableConfigVersion == before.ExperimentConfig && pkg.ExperimentConfigVersion == ""
	}, "promote_experiment_config "+before.ExperimentConfig)
}

func (s *configMacOSSuite) stopConfigExperimentRC() {
	before := s.currentExpectedState()
	s.pushTaskUntil("stop_experiment_config", nil, func(pkg backend.RemoteConfigStatePackage) bool {
		// A stop rolls the experiment back, so the stable configuration is the one it already was.
		return pkg.StableConfigVersion == before.StableConfig && pkg.ExperimentConfigVersion == ""
	}, "stop_experiment_config "+before.ExperimentConfig)
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

// requireResting requires that nothing is deployed, both on disk and in what the daemon reports.
//
// The daemon's report is polled because it can trail the disk: after a watcher revert, /status only
// catches up at the daemon's next state refresh.
func (s *configMacOSSuite) requireResting() {
	require.True(s.T(), s.etcExpResting(), "host must be resting (etc-exp a symlink or absent) before this test starts")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if !assert.NoError(c, err) || !assert.Len(c, status.Packages, 1) {
			return
		}
		assert.Empty(c, status.Packages[0].ExperimentConfigVersion, "the daemon must not report a deployed experiment")
	}, 30*time.Second, 2*time.Second, "host must be resting (no experiment reported by the daemon) before this test starts")
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
	if !s.etcExpResting() {
		s.T().Logf("%s left a configuration experiment deployed; rolling it back to restore the resting state", testName)
		s.restoreResting()
	}
	s.resetStableConfig()
}

const stableConfigBaselinePath = "/var/tmp/e2e-fleet-etc-baseline.tar"

// saveStableConfigBaseline archives the stable configuration the suite starts from, so that
// resetStableConfig can put it back after every test.
//
// Without it, every promote leaves its change in stable configuration for the rest of the run, and
// each test runs against whatever the tests before it promoted.
func (s *configMacOSSuite) saveStableConfigBaseline() {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo tar --acls --xattrs -cpf %[1]s -C /opt/datadog-agent etc && sudo chmod 600 %[1]s", stableConfigBaselinePath))
	require.NoError(s.T(), err, "could not archive the stable configuration")
	s.stableConfigBaseline = s.stableConfigFingerprint()
}

// stableConfigFingerprint is a checksum over the path and content of every file in the stable
// configuration directory.
func (s *configMacOSSuite) stableConfigFingerprint() string {
	out, err := s.Env().RemoteHost.Execute(
		`sudo sh -c 'cd /opt/datadog-agent/etc && find . -type f -print0 | sort -z | xargs -0 shasum | shasum'`)
	require.NoError(s.T(), err)
	return strings.TrimSpace(out)
}

// resetStableConfig puts back the stable configuration archived by saveStableConfigBaseline, when
// a test changed it.
//
// The archive restores etc's .deployment-id with the rest, and the daemon reads its stable config
// version from that file, so the daemon is restarted to report it. Its pending UPDATER_TASKs are
// cleared first: a restarted daemon has no memory of the tasks it already executed, and an early
// task whose expected_state matches the restored configuration again would be re-applied.
func (s *configMacOSSuite) resetStableConfig() {
	if s.stableConfigBaseline == "" || s.stableConfigFingerprint() == s.stableConfigBaseline {
		return
	}
	s.T().Log("the test changed the stable configuration; restoring the suite's baseline")
	s.clearPendingUpdaterTasks()
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`sudo sh -c 'set -e; cd /opt/datadog-agent; `+
		`rm -rf .e2e-etc-restore; mkdir .e2e-etc-restore; tar --acls --xattrs -xpf %s -C .e2e-etc-restore; `+
		`rm -rf etc; mv .e2e-etc-restore/etc etc; rmdir .e2e-etc-restore'`, stableConfigBaselinePath))
	require.NoError(s.T(), err, "could not restore the stable configuration")
	for _, label := range append(append([]string{}, swappableJobLabels...), "com.datadoghq.installer") {
		_, err := s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/" + label)
		require.NoError(s.T(), err, "could not restart %s on the restored configuration", label)
	}
	s.waitForDaemon()
	require.Equal(s.T(), s.stableConfigBaseline, s.stableConfigFingerprint(), "the restored stable configuration differs from the baseline")
}

// launchdLabelsLoaded returns which of the given labels currently appear in `launchctl list`.
//
// Labels are matched exactly against the list's label column: a substring match would count
// com.datadoghq.agent as loaded while only com.datadoghq.agent-exp is.
func (s *configMacOSSuite) launchdLabelsLoaded(labels ...string) map[string]bool {
	out, err := s.Env().RemoteHost.Execute("sudo launchctl list | grep datadoghq || true")
	require.NoError(s.T(), err)
	listed := map[string]bool{}
	for line := range strings.SplitSeq(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			listed[fields[len(fields)-1]] = true
		}
	}
	loaded := make(map[string]bool, len(labels))
	for _, label := range labels {
		loaded[label] = listed[label]
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

// TestConfigFailureCrashMacOS pins that an experiment whose Agent refuses to start is rolled back
// on its own, and that the bad value never reaches the stable configuration.
//
// The experiment sets log_level to an unresolvable ENC[...] placeholder, which agent-exp rejects at
// startup ("unknown log level"). The start itself succeeds -- the configuration is written and the
// experiment job set loaded -- and the watcher (config_experiment_watcher_darwin.go) then reverts
// on agent-exp's exit, within seconds and with no stop task. That window is too short to poll the
// experiment as deployed, so the test asserts on the start task's own result and on the end state.
// TestAgentExpCrashRevertsAndIsNotRelaunchedMacOS covers a crash of an Agent that did start.
func (s *configMacOSSuite) TestConfigFailureCrashMacOS() {
	s.requireResting()

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	stableFingerprint := s.stableConfigFingerprint()
	rejectionsBefore := s.agentExpLogLevelRejections()

	deploymentID := nextID("cfg-crash")
	s.pushInstallerConfig(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "ENC[invalid_secret]"}`)},
	})
	task := s.pushTaskUntilExecuted("start_experiment_config", &experimentTaskParams{Version: deploymentID},
		"start_experiment_config "+deploymentID)
	require.Equal(s.T(), backend.TaskStateDone, task.State, "the experiment should start even though its Agent cannot: %+v", task.Error)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.etcExpResting(), "etc-exp should be a symlink again once the watcher reverts")
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			assert.Empty(c, status.Packages[0].ExperimentConfigVersion, "the daemon should no longer report the experiment")
		}
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s should be unloaded by the revert", label)
		}
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running again")
	}, 60*time.Second, 5*time.Second)

	require.Greater(s.T(), s.agentExpLogLevelRejections(), rejectionsBefore, "agent-exp should have refused the unresolvable log level")

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), before["log_level"], config["log_level"],
		"an unresolvable secret placeholder must not survive the rollback")
	require.Equal(s.T(), stableFingerprint, s.stableConfigFingerprint(), "the rollback must leave the stable configuration untouched")
}

// agentExpLogLevelRejections counts how many times agent-exp has refused TestConfigFailureCrashMacOS's
// log level. launchd-exp.log outlives a test, so callers compare counts rather than look for a match.
func (s *configMacOSSuite) agentExpLogLevelRejections() int {
	out, err := s.Env().RemoteHost.Execute(
		`sudo grep -c 'unknown log level: enc\[invalid_secret\]' /opt/datadog-agent/logs/launchd-exp.log 2>/dev/null || true`)
	require.NoError(s.T(), err)
	count := 0
	_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count
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
// files: _dd-agent/admin mode 0660, the ownership the .dmg's postinstall script gives the
// configuration tree, not Linux's dd-agent/dd-agent.
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
		require.Equal(s.T(), "_dd-agent", perms.Owner, "%s should be owned by _dd-agent", path)
		require.Equal(s.T(), "admin", perms.Group, "%s should have group admin", path)
		require.Equal(s.T(), "660", perms.Mode, "%s should have mode 0660", path)
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

	// network_config.enabled is used rather than runtime_security_config.enabled because it maps
	// to NetworkTracerModule, which has a real factory on macOS (network_tracer_darwin.go).
	// EventMonitorModule (what runtime_security_config.enabled maps to) has no darwin factory —
	// only eventmonitor_linux.go and eventmonitor_windows.go exist — so enabling it makes
	// system-probe exit immediately with "no module could be loaded".
	s.startConfigExperimentRC(nextID("cfg-sysprobe"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config": {"enabled": true}}`)},
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

	// launchd state is polled: the task reports done once the installer returns, and a job's entry
	// in `launchctl list` can trail that by a moment.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded(experimentLabels()...)
		for _, label := range experimentLabels() {
			assert.True(c, loaded[label], "%s should be loaded during the experiment", label)
		}
	}, 30*time.Second, 2*time.Second)
	out, err := s.Env().RemoteHost.Execute("sudo launchctl print system/com.datadoghq.agent-exp | grep -c KeepAlive || true")
	require.NoError(s.T(), err)
	require.Equal(s.T(), "0", strings.TrimSpace(out), "the experiment job set must not carry KeepAlive")

	s.stopConfigExperimentRC()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded(append(append([]string{}, swappableJobLabels...), experimentLabels()...)...)
		for _, label := range swappableJobLabels {
			assert.True(c, loaded[label], "%s should be loaded again after stop", label)
		}
		for _, label := range experimentLabels() {
			assert.False(c, loaded[label], "%s should no longer be loaded after stop", label)
		}
	}, 30*time.Second, 2*time.Second)
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
// pkg/fleet/installer/packages/datadog_agent_darwin.go's installStableJobs: the stable launchd job
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

// --- Set 13: watcher crash-revert regression (macOS-specific) ----------------------------------

// TestWatcherRevertsOnSysprobeCrashMacOS regression-tests the detached watcher added in
// config_experiment_watcher_darwin.go: it supervises every job in a deployed configuration
// experiment and reverts to stable the moment any of them exits abnormally, with no explicit
// stop/promote task ever sent. A SIGKILL'd job reports as Signaled rather than a clean Exited/
// ExitStatus==0, so exitedCleanly treats it as a crash and the watcher's own exit observer -- not
// this test -- drives the revert.
func (s *configMacOSSuite) TestWatcherRevertsOnSysprobeCrashMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-watcher-crash"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config": {"enabled": true}}`)},
	}, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe-exp")
		assert.True(c, loaded["com.datadoghq.sysprobe-exp"], "sysprobe experiment job should be loaded")
	}, 60*time.Second, 5*time.Second)

	_, err := s.Env().RemoteHost.Execute("sudo launchctl kill SIGKILL system/com.datadoghq.sysprobe-exp")
	require.NoError(s.T(), err, "sending SIGKILL to the experiment sysprobe job should succeed")

	// No s.stopConfigExperimentRC() call here: the revert below must come from the watcher's own
	// exit observer noticing the abnormal exit, not from an explicit task this test sends.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.etcExpResting(), "etc-exp should be a symlink again once the watcher reverts")
		assert.Empty(c, s.packageState(s.readStatus()).ExperimentConfigVersion,
			"experiment_config_version should be cleared once the watcher reverts")
	}, 90*time.Second, 5*time.Second)

	loadedStable := s.launchdLabelsLoaded("com.datadoghq.sysprobe")
	require.True(s.T(), loadedStable["com.datadoghq.sysprobe"], "sysprobe stable job should be loaded again after the watcher's revert")

	// Best-effort: the watcher's own log line corroborates *why* it reverted, but the daemon-state
	// assertions above already establish *that* it did, so this is a soft check.
	if out, err := s.Env().RemoteHost.Execute(
		"sudo grep -c 'watcher: reverting configuration experiment' /opt/datadog-agent/logs/updater.log || true"); err == nil {
		s.T().Logf("watcher revert log line occurrences in updater.log: %s", strings.TrimSpace(out))
	} else {
		s.T().Logf("could not read updater.log for the watcher's revert log line: %v", err)
	}
}

// --- Set 14: shutdown-mid-experiment recovery (macOS-specific) ---------------------------------

const installerDaemonPlistPath = "/Library/LaunchDaemons/com.datadoghq.installer.plist"

// killAllServicesSimulatingShutdown simulates an abrupt shutdown (crash, power loss, kill -9)
// during a live configuration experiment: every swappable job, in both variants, and the detached
// watcher process (config_experiment_watcher_darwin.go) that has no launchd job of its own and so
// would otherwise never come back, are killed. The installer daemon itself is booted out rather
// than sent a plain kill, so that its own KeepAlive stanza (RunAtLoad+KeepAlive.SuccessfulExit:
// false in com.datadoghq.installer.plist.tmpl) cannot race this helper by respawning it before the
// caller is ready -- bringing the daemon back up is always a separate, explicit
// restartInstallerDaemon call.
//
// Only the stable job set is restarted here, mirroring what actually comes back unsupervised after
// a real reboot: the experiment job set and the watcher stay down until the daemon's resume-or-
// revert logic (pkg/fleet/installer/packages/config_experiment_resume_darwin.go) decides their fate.
func (s *configMacOSSuite) killAllServicesSimulatingShutdown() {
	_, _ = s.Env().RemoteHost.Execute("sudo pkill -SIGKILL -f 'package-command datadog-agent watchConfigExperiment' || true")

	_, err := s.Env().RemoteHost.Execute("sudo launchctl bootout system/com.datadoghq.installer || true")
	require.NoError(s.T(), err, "booting out the installer daemon should succeed")

	for _, label := range append(append([]string{}, swappableJobLabels...), experimentLabels()...) {
		_, _ = s.Env().RemoteHost.Execute("sudo launchctl kill SIGKILL system/" + label + " || true")
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("pgrep -f 'package-command datadog-agent watchConfigExperiment' || true")
		assert.NoError(c, err)
		assert.Empty(c, strings.TrimSpace(out), "the detached watcher process should be gone")
	}, 30*time.Second, 2*time.Second)

	// The kill loop above only ever reaches a *running* job: by the time this method runs, an
	// active experiment has already had configExperiment.Start bootout the stable set entirely
	// (pkg/fleet/installer/packages/config_experiment_darwin.go), leaving only the -exp variants
	// loaded. So the stable definitions must be reloaded from disk here, the same bootstrap+
	// enable+kickstart sequence restartInstallerDaemon uses below -- a plain kickstart would fail
	// with "Could not find service" against a label launchd has never heard of.
	for _, label := range swappableJobLabels {
		plistPath := "/Library/LaunchDaemons/" + label + ".plist"
		_, err := s.Env().RemoteHost.Execute("sudo launchctl bootstrap system " + plistPath)
		require.NoError(s.T(), err, "bootstrapping the stable %s job should succeed", label)
		_, err = s.Env().RemoteHost.Execute("sudo launchctl enable system/" + label)
		require.NoError(s.T(), err, "enabling the stable %s job should succeed", label)
		_, err = s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/" + label)
		require.NoError(s.T(), err, "restarting the stable %s job should succeed", label)
	}
}

// restartInstallerDaemon brings the installer daemon back up after killAllServicesSimulatingShutdown
// booted it out, mirroring the bootstrap+enable+kickstart sequence launchd.JobSet.Start uses for the
// job sets it owns: bootout leaves no cached definition behind, so a plain kickstart would fail with
// "no such process" until the definition is reloaded from disk.
func (s *configMacOSSuite) restartInstallerDaemon() {
	_, err := s.Env().RemoteHost.Execute("sudo launchctl bootstrap system " + installerDaemonPlistPath)
	require.NoError(s.T(), err, "bootstrapping the installer daemon should succeed")
	_, err = s.Env().RemoteHost.Execute("sudo launchctl enable system/com.datadoghq.installer")
	require.NoError(s.T(), err, "enabling the installer daemon should succeed")
	_, err = s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/com.datadoghq.installer")
	require.NoError(s.T(), err, "kickstarting the installer daemon should succeed")
	s.waitForDaemon()
}

func (s *configMacOSSuite) watcherRunning() bool {
	out, err := s.Env().RemoteHost.Execute("pgrep -f 'package-command datadog-agent watchConfigExperiment' || true")
	require.NoError(s.T(), err)
	return strings.TrimSpace(out) != ""
}

// TestShutdownMidExperimentResumesWatcherMacOS regression-tests
// pkg/fleet/installer/packages/config_experiment_resume_darwin.go: an abrupt shutdown mid-experiment,
// with the experiment's deadline still valid, must be recovered on the daemon's next start by
// re-establishing the experiment job set and relaunching its watcher, not by reverting to stable.
func (s *configMacOSSuite) TestShutdownMidExperimentResumesWatcherMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-shutdown-resume")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config": {"enabled": true}}`)},
	}, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe-exp")
		assert.True(c, loaded["com.datadoghq.sysprobe-exp"], "sysprobe experiment job should be loaded")
	}, 60*time.Second, 5*time.Second)
	require.True(s.T(), s.watcherRunning(), "the watcher process should be running while the experiment is live")

	s.killAllServicesSimulatingShutdown()
	require.False(s.T(), s.watcherRunning(), "the watcher process must be gone before the daemon is asked to resume it")

	s.restartInstallerDaemon()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe-exp")
		assert.True(c, loaded["com.datadoghq.sysprobe-exp"], "sysprobe experiment job should be loaded again after resume")
		assert.True(c, s.watcherRunning(), "a new watcher process should be supervising the resumed experiment")
	}, 60*time.Second, 5*time.Second)

	require.False(s.T(), s.etcExpResting(), "etc-exp should still be a real directory: the experiment must not have been reverted")
	require.Equal(s.T(), deploymentID, s.packageState(s.readStatus()).ExperimentConfigVersion,
		"experiment_config_version should still reflect the resumed experiment, not a revert")

	s.stopConfigExperimentRC()
}

// TestShutdownAfterDeadlineExpiryRevertsMacOS regression-tests
// pkg/fleet/installer/packages/config_experiment_resume_darwin.go: an abrupt shutdown mid-experiment
// whose deadline has already expired by the time the daemon restarts must be reverted to stable, not
// resumed -- the unsupervised window guaranteed by configExperimentDeadlineWindow was exceeded before
// the daemon got a chance to recover it. The deadline file is forced into the past directly on disk
// (rather than waiting out the real window) while the daemon is down, so there is no race with the
// daemon reading it.
func (s *configMacOSSuite) TestShutdownAfterDeadlineExpiryRevertsMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-shutdown-expire")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config": {"enabled": true}}`)},
	}, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe-exp")
		assert.True(c, loaded["com.datadoghq.sysprobe-exp"], "sysprobe experiment job should be loaded")
	}, 60*time.Second, 5*time.Second)

	s.killAllServicesSimulatingShutdown()

	_, err := s.Env().RemoteHost.Execute(
		`sudo sh -c 'date -u -v-2H +"%Y-%m-%dT%H:%M:%SZ" > /opt/datadog-agent/run/experiment-deadline'`)
	require.NoError(s.T(), err, "forcing the experiment deadline into the past should succeed")

	// See clearPendingUpdaterTasks: without this, the start_experiment_config task pushed above gets
	// redelivered right after restart and re-applied, undoing the revert this test is asserting on.
	s.clearPendingUpdaterTasks()

	s.restartInstallerDaemon()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.etcExpResting(), "etc-exp should be a symlink again: an already-expired deadline must be reverted, not resumed")
		assert.Empty(c, s.packageState(s.readStatus()).ExperimentConfigVersion,
			"experiment_config_version should be cleared once the expired deadline is reverted")
	}, 60*time.Second, 5*time.Second)

	loadedStable := s.launchdLabelsLoaded("com.datadoghq.sysprobe")
	require.True(s.T(), loadedStable["com.datadoghq.sysprobe"], "sysprobe stable job should be loaded again after the revert")
	require.False(s.T(), s.watcherRunning(), "no watcher process should be left running for a reverted experiment")
}

// --- Set 15: daemon socket, watcher lifecycle and upgrades (macOS-specific) --------------------

const (
	installerSocketPath    = "/opt/datadog-agent/run/installer.sock"
	experimentDeadlinePath = "/opt/datadog-agent/run/experiment-deadline"
)

// socketRequestAs calls the installer daemon's local API as user and returns the HTTP status code it
// answered with.
func (s *configMacOSSuite) socketRequestAs(user, method, path string) string {
	out, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`sudo -u %s curl -sS -o /dev/null -w '%%{http_code}' -X %s -H 'Content-Type: application/json' --unix-socket %s http://installer%s`,
		user, method, installerSocketPath, path))
	require.NoError(s.T(), err, "%s should be able to connect to the installer socket", user)
	return strings.TrimSpace(out)
}

// agentSocketRequest calls the installer daemon's local API as the Agent's account.
func (s *configMacOSSuite) agentSocketRequest(method, path string) string {
	return s.socketRequestAs("_dd-agent", method, path)
}

// rootOnlyRoutes is every route pkg/fleet/daemon/local_api.go serves other than GET /status, with
// the package path parameter filled in.
var rootOnlyRoutes = []struct{ method, path string }{
	{"POST", "/catalog"},
	{"POST", "/config_catalog"},
	{"POST", "/datadog-agent/experiment/start"},
	{"POST", "/datadog-agent/experiment/stop"},
	{"POST", "/datadog-agent/experiment/promote"},
	{"POST", "/datadog-agent/config_experiment/start"},
	{"POST", "/datadog-agent/config_experiment/stop"},
	{"POST", "/datadog-agent/config_experiment/promote"},
	{"POST", "/datadog-agent/install"},
	{"POST", "/datadog-agent/remove"},
	{"GET", "/debug/pprof/"},
	{"GET", "/debug/pprof/cmdline"},
}

// TestInstallerSocketReservesChangesForRootMacOS pins pkg/fleet/daemon/local_api_unix.go and
// local_api.go's requireRootForChanges on macOS: the socket belongs to _dd-agent so that
// `datadog-agent status` can read the daemon's status, which leaves the per-route check as the
// only thing keeping that account from driving the installer.
//
// Every route is checked, pprof included: pprof is mounted outside the API sub-mux and its
// Content-Type check, so the root check is the only thing in front of it.
func (s *configMacOSSuite) TestInstallerSocketReservesChangesForRootMacOS() {
	s.requireResting()

	perms, err := s.Host.GetFilePermissions(installerSocketPath)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "_dd-agent", perms.Owner, "the installer socket should be handed to the Agent's account")
	require.Equal(s.T(), "700", perms.Mode, "the installer socket must be owner-only")

	// The socket's mode is what keeps everyone else out: they cannot connect at all.
	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf(
		`sudo -u nobody curl -sS -o /dev/null -H 'Content-Type: application/json' --unix-socket %s http://installer/status`,
		installerSocketPath))
	require.Error(s.T(), err, "an account other than root and _dd-agent must not be able to connect to the installer socket")

	require.Equal(s.T(), "200", s.socketRequestAs("root", "GET", "/status"), "root should be able to read the daemon's status")
	require.Equal(s.T(), "200", s.agentSocketRequest("GET", "/status"),
		"the Agent's account should be able to read the daemon's status")
	for _, route := range rootOnlyRoutes {
		assert.Equal(s.T(), "403", s.agentSocketRequest(route.method, route.path),
			"the Agent's account must not be able to call %s %s", route.method, route.path)
	}
}

// watcherCount returns how many experiment watcher processes are running.
func (s *configMacOSSuite) watcherCount() int {
	out, err := s.Env().RemoteHost.Execute("pgrep -f 'package-command datadog-agent watchConfigExperiment' | wc -l")
	require.NoError(s.T(), err)
	count := 0
	_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count
}

// requireWatcherExits waits for every watcher process to be gone. The watcher notices that its
// experiment is over on its next deadline tick (deadlineTickInterval, 30s), so the timeout leaves
// room for a full tick plus the stop itself.
func (s *configMacOSSuite) requireWatcherExits(after string) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Zero(c, s.watcherCount(), "the watcher should exit once its experiment is %s", after)
	}, 90*time.Second, 5*time.Second)
}

// TestWatcherExitsWithItsExperimentMacOS regression-tests
// pkg/fleet/installer/packages/config_experiment_watcher_darwin.go: a deliberate stop or promote
// lets the experiment jobs exit cleanly, which gives the watcher no exit event to act on, so it has
// to notice on its own that its experiment is over. A watcher that did not would keep polling for
// good, and repeated deployments would pile up processes acting on each other's deadline.
func (s *configMacOSSuite) TestWatcherExitsWithItsExperimentMacOS() {
	s.requireResting()

	end := map[string]func(){
		"stopped":  s.stopConfigExperimentRC,
		"promoted": s.promoteConfigExperimentRC,
	}
	for _, how := range []string{"stopped", "stopped", "promoted"} {
		s.startConfigExperimentRC(nextID("cfg-watcher-"+how), []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		}, nil)
		require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
			assert.Equal(c, 1, s.watcherCount(), "exactly one watcher should supervise the live experiment")
		}, 30*time.Second, 2*time.Second)

		end[how]()
		s.requireWatcherExits(how)
	}
}

// TestUpgradeMidExperimentClearsDeadlineMacOS regression-tests
// omnibus/package-scripts/agent-dmg/preinst: an upgrade collapses a live configuration experiment
// back onto the stable configuration, and must take the experiment's deadline with it. A deadline
// left behind still inside its window has the next daemon resume the experiment job set on top of
// the stable configuration, from resumeConfigExperimentDatadogAgent.
//
// The upgrade is a reinstall of the same .dmg over the running host, which is what runs preinst's
// existing-installation path. It is the slowest test in the suite for that reason.
func (s *configMacOSSuite) TestUpgradeMidExperimentClearsDeadlineMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-upgrade"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	_, err := s.Env().RemoteHost.Execute("sudo test -f " + experimentDeadlinePath)
	require.NoError(s.T(), err, "a live experiment should have a deadline")

	// See clearPendingUpdaterTasks: the reinstall drops the daemon's Remote Config state, so without
	// this the start_experiment_config task above is redelivered and deploys the experiment again.
	s.clearPendingUpdaterTasks()

	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithRemoteConfig())
	s.waitForDaemon()

	_, err = s.Env().RemoteHost.Execute("sudo test ! -e " + experimentDeadlinePath)
	require.NoError(s.T(), err, "the upgrade must clear the deadline of the experiment it discarded")
	require.True(s.T(), s.etcExpResting(), "the upgrade should collapse etc-exp back onto etc")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Empty(c, s.packageState(s.readStatus()).ExperimentConfigVersion,
			"the daemon must not report the discarded experiment")
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s must not be resumed after the upgrade", label)
		}
		loaded := s.launchdLabelsLoaded("com.datadoghq.agent")
		assert.True(c, loaded["com.datadoghq.agent"], "the stable Agent job should be running after the upgrade")
	}, 60*time.Second, 5*time.Second)
	s.requireWatcherExits("discarded by the upgrade")
}

// --- Set 16: on-disk and launchd invariants of the experiment lifecycle (macOS-specific) --------

const installerDaemonLabel = "com.datadoghq.installer"

// jobPID returns the pid launchd reports for label, or "" when the job is not loaded or has no
// running process.
func (s *configMacOSSuite) jobPID(label string) string {
	out, err := s.Env().RemoteHost.Execute(
		"sudo launchctl print system/" + label + ` 2>/dev/null | awk '$1 == "pid" && $2 == "=" { print $3; exit }' || true`)
	require.NoError(s.T(), err)
	return strings.TrimSpace(out)
}

// pathExists reports whether path exists on the host, without following a final symlink.
func (s *configMacOSSuite) pathExists(path string) bool {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf("sudo test -e %[1]s || sudo test -L %[1]s", path))
	return err == nil
}

// readPlist returns the launchd job definition at path, converted to JSON by plutil.
func (s *configMacOSSuite) readPlist(path string) map[string]any {
	out, err := s.Env().RemoteHost.Execute("sudo plutil -convert json -o - " + path)
	require.NoError(s.T(), err, "could not read %s", path)
	var plist map[string]any
	require.NoError(s.T(), json.Unmarshal([]byte(out), &plist), "could not parse %s", path)
	return plist
}

// remoteNow returns the host's clock, so that times written by the daemon are compared against the
// clock that wrote them rather than the test runner's.
func (s *configMacOSSuite) remoteNow() time.Time {
	out, err := s.Env().RemoteHost.Execute(`date -u +%Y-%m-%dT%H:%M:%SZ`)
	require.NoError(s.T(), err)
	now, err := time.Parse(time.RFC3339, strings.TrimSpace(out))
	require.NoError(s.T(), err)
	return now
}

// inode returns the inode number of path, not following a final symlink.
func (s *configMacOSSuite) inode(path string) string {
	out, err := s.Env().RemoteHost.Execute("sudo stat -f %i " + path)
	require.NoError(s.T(), err)
	return strings.TrimSpace(out)
}

func experimentPlistPath(label string) string {
	return "/Library/LaunchDaemons/" + label + "-exp.plist"
}

// TestFailedConfigStartLeavesNoTraceMacOS pins pkg/fleet/installer/config/config_darwin.go's
// WriteExperiment: a start the installer rejects -- here a file operation on a path that is not
// allowed (config.go) -- must leave the host exactly as it was. Nothing is published to etc-exp, no
// scratch directory is left behind, no job is swapped and no deadline is armed.
func (s *configMacOSSuite) TestFailedConfigStartLeavesNoTraceMacOS() {
	s.requireResting()

	stableFingerprint := s.stableConfigFingerprint()
	stableAgentPID := s.jobPID("com.datadoghq.agent")
	require.NotEmpty(s.T(), stableAgentPID, "the stable Agent should be running before the test starts")

	deploymentID := nextID("cfg-rejected")
	s.pushInstallerConfig(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/not-allowed.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	})
	task := s.pushTaskExpectingFailure("start_experiment_config", &experimentTaskParams{Version: deploymentID},
		"start_experiment_config "+deploymentID)
	require.NotNil(s.T(), task.Error, "the failed task should carry its error")
	require.Contains(s.T(), task.Error.Message, "not allowed", "the task should fail on the disallowed path, not for another reason")

	require.True(s.T(), s.etcExpResting(), "a rejected start must not publish etc-exp")
	out, err := s.Env().RemoteHost.Execute("ls -A /opt/datadog-agent | grep '^\\.datadog-config-incoming' || true")
	require.NoError(s.T(), err)
	require.Empty(s.T(), strings.TrimSpace(out), "a rejected start must not leave its scratch directory behind")
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "a rejected start must not arm a deadline")
	for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
		require.False(s.T(), loaded, "a rejected start must not load %s", label)
	}
	require.Equal(s.T(), stableAgentPID, s.jobPID("com.datadoghq.agent"), "a rejected start must not restart the stable Agent")
	require.Equal(s.T(), stableFingerprint, s.stableConfigFingerprint(), "a rejected start must not touch the stable configuration")
	require.Empty(s.T(), s.packageState(s.readStatus()).ExperimentConfigVersion, "a rejected start must not be reported as deployed")
}

// TestExperimentJobDefinitionsMacOS pins the experiment job set in
// pkg/fleet/installer/packages/embedded/tmpl/gen/darwin: each -exp job runs the same program as its
// stable twin, under the same account, reading etc-exp instead of etc; it is never relaunched by
// launchd (no KeepAlive) and never started at boot (RunAtLoad false); and its definition is
// written root:wheel 0644 and removed again once the experiment ends.
func (s *configMacOSSuite) TestExperimentJobDefinitionsMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-exp-jobdefs"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)

	for _, label := range swappableJobLabels {
		path := experimentPlistPath(label)
		perms, err := s.Host.GetFilePermissions(path)
		require.NoError(s.T(), err)
		require.Equal(s.T(), [3]string{"644", "root", "wheel"}, [3]string{perms.Mode, perms.Owner, perms.Group},
			"%s should be root:wheel 0644", path)

		experiment := s.readPlist(path)
		stable := s.readPlist("/Library/LaunchDaemons/" + label + ".plist")
		require.NotContains(s.T(), experiment, "KeepAlive", "%s must not be relaunched by launchd", path)
		require.Equal(s.T(), false, experiment["RunAtLoad"], "%s must not start at boot", path)
		require.Equal(s.T(), stable["UserName"], experiment["UserName"], "%s should run as its stable twin's account", path)
		require.Equal(s.T(), stable["GroupName"], experiment["GroupName"], "%s should run as its stable twin's group", path)

		// Mapping etc-exp back to etc must give exactly the stable job's command line: the same
		// program, with only the configuration directory swapped.
		experimentArgs, ok := experiment["ProgramArguments"].([]any)
		require.True(s.T(), ok, "%s should have ProgramArguments", path)
		var mapped []any
		var readsExperiment bool
		for _, arg := range experimentArgs {
			str, _ := arg.(string)
			readsExperiment = readsExperiment || strings.Contains(str, "/opt/datadog-agent/etc-exp")
			mapped = append(mapped, strings.ReplaceAll(str, "/opt/datadog-agent/etc-exp", "/opt/datadog-agent/etc"))
		}
		require.True(s.T(), readsExperiment, "%s should read its configuration from etc-exp", path)
		require.Equal(s.T(), stable["ProgramArguments"], mapped, "%s should run its stable twin's program on etc-exp", path)

		env, _ := experiment["EnvironmentVariables"].(map[string]any)
		require.NotContains(s.T(), env, "DD_FLEET_POLICIES_DIR", "%s must not point policies outside the configuration directory", path)
		if label == "com.datadoghq.agent" {
			require.Equal(s.T(), "/opt/datadog-agent/etc-exp/conf.d", env["DD_CONFD_PATH"], "%s should load checks from etc-exp", path)
		}
	}

	s.stopConfigExperimentRC()

	for _, label := range swappableJobLabels {
		require.False(s.T(), s.pathExists(experimentPlistPath(label)), "%s should be removed once the experiment ends", experimentPlistPath(label))
		_, err := s.Env().RemoteHost.Execute("sudo launchctl print system/" + label + "-exp")
		require.Error(s.T(), err, "%s-exp should be gone from launchd once the experiment ends", label)
	}
}

// TestAgentExpCrashRevertsAndIsNotRelaunchedMacOS covers the RFC's first failure mode, "the -exp
// Agent crashes": launchd does not relaunch it, since the experiment set has no KeepAlive, and the
// watcher (config_experiment_watcher_darwin.go) reverts to stable on its exit.
// TestWatcherRevertsOnSysprobeCrashMacOS covers the same path for system-probe.
func (s *configMacOSSuite) TestAgentExpCrashRevertsAndIsNotRelaunchedMacOS() {
	s.requireResting()

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)

	s.startConfigExperimentRC(nextID("cfg-agent-crash"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)

	var crashedPID string
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		crashedPID = s.jobPID("com.datadoghq.agent-exp")
		assert.NotEmpty(c, crashedPID, "the experiment Agent should be running")
	}, 30*time.Second, 2*time.Second)
	_, err = s.Env().RemoteHost.Execute("sudo kill -9 " + crashedPID)
	require.NoError(s.T(), err)

	// Until the revert removes the job, launchd must not have started another process for it.
	for range 5 {
		pid := s.jobPID("com.datadoghq.agent-exp")
		require.True(s.T(), pid == "" || pid == crashedPID, "launchd relaunched the crashed experiment Agent as pid %s", pid)
		time.Sleep(time.Second)
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.etcExpResting(), "etc-exp should be a symlink again once the watcher reverts")
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			assert.Empty(c, status.Packages[0].ExperimentConfigVersion, "the daemon should no longer report the experiment")
		}
		assert.False(c, s.pathExists(experimentDeadlinePath), "the revert should clear the deadline")
		assert.False(c, s.pathExists(experimentPlistPath("com.datadoghq.agent")), "the revert should remove the experiment job definitions")
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running again")
	}, 60*time.Second, 5*time.Second)

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), before["log_level"], config["log_level"], "the stable Agent should read the stable configuration")
}

// TestDeadlineFileLifecycleMacOS pins the persisted deadline (pkg/fleet/installer/packages/launchd/
// supervisor.go, config_experiment_darwin.go): it exists only while an experiment is live, belongs
// to root, and holds a time configExperimentDeadlineWindow (60 minutes) after the start. It is what
// the watcher and the next daemon start judge the experiment window by.
func (s *configMacOSSuite) TestDeadlineFileLifecycleMacOS() {
	s.requireResting()
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "a resting host should have no deadline")

	end := map[string]func(){
		"promoted": s.promoteConfigExperimentRC,
		"stopped":  s.stopConfigExperimentRC,
	}
	for _, how := range []string{"promoted", "stopped"} {
		started := s.remoteNow()
		s.startConfigExperimentRC(nextID("cfg-deadline-"+how), []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		}, nil)

		perms, err := s.Host.GetFilePermissions(experimentDeadlinePath)
		require.NoError(s.T(), err, "a live experiment should have a deadline")
		require.Equal(s.T(), [3]string{"644", "root", "wheel"}, [3]string{perms.Mode, perms.Owner, perms.Group},
			"the deadline should be root:wheel 0644, so the experiment's own account cannot rewrite it")

		deadline, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s.readFile(experimentDeadlinePath)))
		require.NoError(s.T(), err, "the deadline should hold an RFC 3339 time")
		require.WithinRange(s.T(), deadline, started.Add(59*time.Minute), s.remoteNow().Add(61*time.Minute),
			"the deadline should be 60 minutes after the start")

		end[how]()
		require.False(s.T(), s.pathExists(experimentDeadlinePath), "the deadline should be cleared once the experiment is %s", how)
	}
}

// TestDaemonRestartResumesExperimentMacOS pins pkg/fleet/installer/packages/
// config_experiment_resume_darwin.go on the plain restart path: restarting the installer daemon in
// the middle of an experiment resumes it -- the experiment Agent runs, exactly one watcher
// supervises it -- and leaves the experiment window where it was. TestShutdownMidExperimentResumes
// WatcherMacOS covers the same code after a hard kill of every job.
func (s *configMacOSSuite) TestDaemonRestartResumesExperimentMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-daemon-restart")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Equal(c, 1, s.watcherCount(), "exactly one watcher should supervise the live experiment")
	}, 30*time.Second, 2*time.Second)
	deadline := s.readFile(experimentDeadlinePath)

	// See clearPendingUpdaterTasks: a restarted daemon has no memory of the tasks it executed.
	s.clearPendingUpdaterTasks()
	_, err := s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/" + installerDaemonLabel)
	require.NoError(s.T(), err)
	s.waitForDaemon()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent-exp"), "the experiment Agent should run after the daemon restarts")
		assert.Equal(c, 1, s.watcherCount(), "exactly one watcher should supervise the resumed experiment")
	}, 60*time.Second, 5*time.Second)
	require.Equal(s.T(), deadline, s.readFile(experimentDeadlinePath), "resuming must not move the experiment window")
	require.False(s.T(), s.etcExpResting(), "the experiment must not be reverted by a daemon restart")
	require.Equal(s.T(), deploymentID, s.packageState(s.readStatus()).ExperimentConfigVersion,
		"the daemon should still report the experiment after restarting")

	s.stopConfigExperimentRC()
	s.requireWatcherExits("stopped")
}

// TestOrphanExperimentWithoutDeadlineRevertsMacOS pins the orphan branch of pkg/fleet/installer/
// packages/config_experiment_resume_darwin.go: an experiment still deployed in etc-exp with no
// deadline left to bound it is reverted the next time the daemon starts. Removing the deadline
// tells the watcher a deliberate end has begun, so it exits without reverting and leaves exactly
// that orphan behind.
func (s *configMacOSSuite) TestOrphanExperimentWithoutDeadlineRevertsMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-orphan"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	_, err := s.Env().RemoteHost.Execute("sudo rm " + experimentDeadlinePath)
	require.NoError(s.T(), err)
	s.requireWatcherExits("left without a deadline")
	require.False(s.T(), s.etcExpResting(), "the experiment should still be deployed once its watcher has exited")

	s.clearPendingUpdaterTasks()
	_, err = s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/" + installerDaemonLabel)
	require.NoError(s.T(), err)
	s.waitForDaemon()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.etcExpResting(), "the daemon should revert an experiment that has no deadline")
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			assert.Empty(c, status.Packages[0].ExperimentConfigVersion, "the daemon should no longer report the experiment")
		}
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s should be unloaded by the revert", label)
		}
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running again")
	}, 60*time.Second, 5*time.Second)
	require.Zero(s.T(), s.watcherCount(), "no watcher should be left for a reverted experiment")
}

// TestInstallerDaemonNotSwappedMacOS pins that the installer daemon is never part of the job-set
// swap (config_experiment_darwin.go's swappable labels): it executes every transition and must keep
// running through all of them.
func (s *configMacOSSuite) TestInstallerDaemonNotSwappedMacOS() {
	s.requireResting()

	daemonPID := s.jobPID(installerDaemonLabel)
	require.NotEmpty(s.T(), daemonPID, "the installer daemon should be running")
	requireSameDaemon := func(after string) {
		require.Equal(s.T(), daemonPID, s.jobPID(installerDaemonLabel), "the installer daemon must keep running after %s", after)
	}

	patch := []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}
	s.startConfigExperimentRC(nextID("cfg-daemon-pid-promote"), patch, nil)
	requireSameDaemon("start")
	s.promoteConfigExperimentRC()
	requireSameDaemon("promote")
	s.startConfigExperimentRC(nextID("cfg-daemon-pid-stop"), patch, nil)
	requireSameDaemon("a second start")
	s.stopConfigExperimentRC()
	requireSameDaemon("stop")
}

// TestPromoteSwapsTheExperimentDirectoryMacOS pins promote and stop at the directory level
// (pkg/fleet/installer/config/config_darwin.go, dir_swap.go): promote moves the experiment
// directory itself onto etc, deployment ID included, rather than copying it, and stop leaves etc
// untouched. It does not tell the atomic swap apart from a pair of renames; the unit tests cover
// that.
func (s *configMacOSSuite) TestPromoteSwapsTheExperimentDirectoryMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-swap")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	require.Equal(s.T(), deploymentID, strings.TrimSpace(s.readFile("/opt/datadog-agent/etc-exp/.deployment-id")),
		"etc-exp should record the deployment it holds")
	experimentInode := s.inode("/opt/datadog-agent/etc-exp")

	s.promoteConfigExperimentRC()
	require.Equal(s.T(), experimentInode, s.inode("/opt/datadog-agent/etc"), "promote should move the experiment directory onto etc")
	require.Equal(s.T(), deploymentID, strings.TrimSpace(s.readFile("/opt/datadog-agent/etc/.deployment-id")),
		"etc should record the promoted deployment")
	require.Equal(s.T(), deploymentID, s.packageState(s.readStatus()).StableConfigVersion,
		"the daemon should report the promoted deployment as stable")

	stableFingerprint := s.stableConfigFingerprint()
	s.startConfigExperimentRC(nextID("cfg-swap-stop"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "warn"}`)},
	}, nil)
	s.stopConfigExperimentRC()
	require.Equal(s.T(), stableFingerprint, s.stableConfigFingerprint(), "stop must leave the stable configuration untouched")
}

// TestVersionMethodsFailMacOS pins that version tasks fail on macOS rather than being reported as
// done, and that nothing on the host changes. The daemon executes them like any other task; they
// fail on the catalog lookup or the download, and a version experiment that got further would be
// refused by the Agent package's preStartExperiment hook
// (pkg/fleet/installer/packages/datadog_agent_darwin.go).
func (s *configMacOSSuite) TestVersionMethodsFailMacOS() {
	s.requireResting()

	before := s.packageState(s.readStatus())
	packagesBefore, err := s.Env().RemoteHost.Execute("ls -A /opt/datadog-packages/datadog-agent")
	require.NoError(s.T(), err)

	for _, method := range []string{"install_package", "start_experiment"} {
		task := s.pushTaskExpectingFailure(method, &experimentTaskParams{Version: "0.0.0"}, method+" on macOS")
		require.Equal(s.T(), backend.TaskStateError, task.State, "%s should be reported as failed", method)

		after := s.packageState(s.readStatus())
		require.Equal(s.T(), before.StableVersion, after.StableVersion, "a failed %s must not change the stable version", method)
		require.Equal(s.T(), before.ExperimentVersion, after.ExperimentVersion, "a failed %s must not start a version experiment", method)
	}

	packagesAfter, err := s.Env().RemoteHost.Execute("ls -A /opt/datadog-packages/datadog-agent")
	require.NoError(s.T(), err)
	require.Equal(s.T(), packagesBefore, packagesAfter, "a failed task must not install anything")
	for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
		require.False(s.T(), loaded, "a failed task must not load %s", label)
	}
}

// TestExperimentCopyPreservesMetadataMacOS pins pkg/fleet/installer/config/config_tree.go: the
// experiment is a copy of etc that keeps every unpatched entry's mode, owner, group and ACL, while
// the files a Fleet operation writes take the .dmg postinstall's _dd-agent:admin 0660.
func (s *configMacOSSuite) TestExperimentCopyPreservesMetadataMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-copy-metadata"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)

	// One line per entry with its type, mode, uid and gid, followed by its ACL entries, for every
	// entry the experiment did not write. Timestamps and sizes are left out: a copy does not keep
	// the former, and the comparison is about metadata rather than content. The "inherited" marker
	// is dropped from ACL entries: config_tree.go's copyACL keeps an inherited entry as an explicit
	// one, which grants the same access, because chmod rejects the marker.
	metadata := func(dir string) string {
		out, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`sudo sh -c 'cd %s && `+
			`find . -mindepth 1 ! -path ./datadog.yaml ! -path ./.deployment-id | sort | while read -r f; do `+
			`printf "%%s %%s\n" "$f" "$(stat -f "%%HT %%Lp %%u %%g" "$f")"; ls -lde "$f" | tail -n +2 | sed "s/ inherited / /"; done'`, dir))
		require.NoError(s.T(), err)
		return out
	}
	stable := metadata("/opt/datadog-agent/etc")
	require.NotEmpty(s.T(), strings.TrimSpace(stable), "the stable configuration should hold more than datadog.yaml")
	require.Equal(s.T(), stable, metadata("/opt/datadog-agent/etc-exp"),
		"the experiment should keep the mode, owner, group and ACL of every entry it did not write")

	perms, err := s.Host.GetFilePermissions("/opt/datadog-agent/etc-exp/datadog.yaml")
	require.NoError(s.T(), err)
	require.Equal(s.T(), [3]string{"660", "_dd-agent", "admin"}, [3]string{perms.Mode, perms.Owner, perms.Group},
		"a file the experiment wrote should be _dd-agent:admin 0660")

	s.stopConfigExperimentRC()
}
