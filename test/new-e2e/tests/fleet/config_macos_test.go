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
	"os"
	"path/filepath"
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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/host/byohost"
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
// CLI path skips the verifyState reconciliation that only Remote Config exercises. Consequently
// this suite does not embed suite.FleetSuite or use its Backend: FleetSuite's dispatch methods are
// CLI-only by design, and this suite never calls them.
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
func TestFleetConfigMacOS(t *testing.T) {
	// LOCAL_VM targets a pre-existing macOS host (a local machine or another box reachable over
	// SSH) instead of provisioning a real AWS Dedicated Host. Its fakeintake serves Remote Config,
	// which every state change in this suite goes through.
	if provisioner, ok := byohost.LocalVMProvisioner(t, byohost.WithRemoteConfig(), byohost.WithDescriptorName("macos-fleet-config-byo-host.json")); ok {
		e2e.Run(t, newConfigMacOSSuite(), e2e.WithProvisioner(provisioner))
		return
	}

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

	// Asserts the shipped .dmg gave the Agent its package entry, which the whole suite depends on:
	// InstallConfigExperiment reads the package's repository unconditionally, and
	// ConfigAndPackageStates enumerates every registered package to answer /status. Nothing here
	// creates it -- `installer postinst datadog-agent dmg` does (linkPackageEntry) -- so a failure
	// means that regressed, not that the suite is misconfigured.
	_, err := s.Env().RemoteHost.Execute(
		"test -L /opt/datadog-packages/datadog-agent/stable && test -L /opt/datadog-packages/datadog-agent/experiment")
	require.NoError(s.T(), err, "the .dmg install did not give the Agent its package entry "+
		"(linkPackageEntry has not run); no configuration experiment can start")

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

// readStatus is tryReadStatus for reads outside a polling loop.
//
// It retries until the daemon answers: a start, stop or promote swaps the installer daemon between
// its stable and experiment jobs shortly after reporting the outcome, and for the ExitTimeOut of
// the job it leaves no daemon is listening on the socket.
func (s *configMacOSSuite) readStatus() backend.RemoteConfigState {
	var status backend.RemoteConfigState
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		var err error
		status, err = s.tryReadStatus()
		assert.NoError(c, err)
	}, 60*time.Second, 2*time.Second, "the installer daemon did not answer on its socket")
	return status
}

func (s *configMacOSSuite) packageState(status backend.RemoteConfigState) backend.RemoteConfigStatePackage {
	require.Len(s.T(), status.Packages, 1, "expected exactly one package in remote config state")
	return status.Packages[0]
}

// currentExpectedState builds an expected_state that matches the daemon's real state right now, so
// a task built from it passes verifyState. /status's values must be copied verbatim -- including
// the literal "empty" stable_config_version substitution -- never translated, since verifyState
// compares them byte for byte.
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

// Like Fleet's backend, it appends a merge patch setting config_id to the deployment ID in
// datadog.yaml, which the installer daemon then reports as the running configuration
// (CreateConfig in dd-source's installer_config_client.go).
func (s *configMacOSSuite) pushInstallerConfig(deploymentID string, fileOps []backend.FileOperation) {
	configID, err := json.Marshal(map[string]string{"config_id": deploymentID})
	require.NoError(s.T(), err)
	fileOps = append(append([]backend.FileOperation{}, fileOps...), backend.FileOperation{
		FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: configID,
	})
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
			s.clearPendingUpdaterTasks()
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
			// See clearPendingUpdaterTasks: every start, stop and promote restarts the installer
			// daemon, and the new process has no memory of the task it just applied.
			s.clearPendingUpdaterTasks()
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

// requireFailedOnHost waits for the end state of an experiment that failed on the host after its
// start was reported (configExperiment.Fail): every job back on its stable job, the stable
// installer daemon included, the deadline replaced by the failed marker, and the experiment still
// deployed and reported, with the stable daemon's config_id as the running configuration. That is
// what makes Fleet's health check fail and roll the experiment back with a stop.
func (s *configMacOSSuite) requireFailedOnHost(deploymentID string) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s should be unloaded by the revert", label)
		}
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running again")
		assert.True(c, s.stableInstallerDaemonLoaded(), "the stable installer daemon should be back")
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			pkg := status.Packages[0]
			assert.Equal(c, deploymentID, pkg.ExperimentConfigVersion, "the failed experiment should still be reported, for Fleet to stop")
			assert.NotEqual(c, deploymentID, pkg.RunningConfigVersion, "the stable daemon should report the stable configuration as running")
		}
	}, 90*time.Second, 5*time.Second)
	require.False(s.T(), s.etcExpResting(), "the failed experiment's configuration should be kept for Fleet's stop")
	require.Equal(s.T(), "failed", strings.TrimSpace(s.readFile(experimentDeadlinePath)), "the deadline should hold the failed marker")
}

// stopFailedExperimentRC sends the stop Fleet sends once its health check fails, and checks it
// clears the failed experiment without restarting the stable jobs, which are already back.
func (s *configMacOSSuite) stopFailedExperimentRC() {
	agentPID := s.jobPID("com.datadoghq.agent")
	s.stopConfigExperimentRC()
	require.True(s.T(), s.etcExpResting(), "the stop should discard the failed experiment's configuration")
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "the stop should clear the failed marker")
	require.Equal(s.T(), agentPID, s.jobPID("com.datadoghq.agent"), "the stop must not restart the stable Agent")
}

// requireUnfinishedOnHost waits for the state a shutdown leaves an experiment in when the watcher
// did not get to fail it first: the host back on the stable job set, as launchd starts it at boot,
// no watcher, and the experiment still deployed and reported, with the stable daemon's config_id as
// the running configuration, for Fleet's health check to stop. Nothing at start settles it, so the
// deadline the previous daemon left is still there.
func (s *configMacOSSuite) requireUnfinishedOnHost(deploymentID string) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running")
		assert.True(c, s.stableInstallerDaemonLoaded(), "the stable installer daemon should be running")
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			pkg := status.Packages[0]
			assert.Equal(c, deploymentID, pkg.ExperimentConfigVersion, "the unfinished experiment should still be reported, for Fleet to stop")
			assert.NotEqual(c, deploymentID, pkg.RunningConfigVersion, "the stable daemon should report the stable configuration as running")
		}
	}, 90*time.Second, 5*time.Second)
	require.Zero(s.T(), s.watcherCount(), "nothing should launch a watcher on a daemon start")
	require.False(s.T(), s.etcExpResting(), "the experiment's configuration should be kept for Fleet's stop")
	deadline := strings.TrimSpace(s.readFile(experimentDeadlinePath))
	require.NotEmpty(s.T(), deadline, "the deadline the previous daemon left should still be there")
	require.NotEqual(s.T(), "failed", deadline, "nothing should have failed the experiment on start")
}

// stopUnfinishedExperimentRC sends Fleet's stop for an experiment a shutdown left unfinished, and
// checks it ends it without restarting the stable Agent launchd already started: the experiment
// configuration discarded, the deadline cleared, and every experiment job, the experiment daemon's
// included, unloaded and its definition removed.
func (s *configMacOSSuite) stopUnfinishedExperimentRC() {
	agentPID := s.jobPID("com.datadoghq.agent")
	s.stopConfigExperimentRC()
	require.True(s.T(), s.etcExpResting(), "the stop should discard the experiment's configuration")
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "the stop should clear the deadline")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s should be unloaded by the stop", label)
		}
	}, 30*time.Second, 2*time.Second)
	require.False(s.T(), s.pathExists("/Library/LaunchDaemons/"+installerDaemonExperimentLabel+".plist"),
		"the experiment daemon's definition should be removed")
	require.Equal(s.T(), agentPID, s.jobPID("com.datadoghq.agent"), "the stop must not restart the stable Agent")
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

// requireResting requires that nothing is deployed, both on disk and in what the daemon reports,
// and that the stable installer daemon is the one running.
//
// The daemon's report is polled because it can trail the disk: after a watcher revert, /status only
// catches up at the daemon's next state refresh. The daemon itself is polled because a stop or
// promote hands it back asynchronously, after reporting its outcome.
func (s *configMacOSSuite) requireResting() {
	require.True(s.T(), s.etcExpResting(), "host must be resting (etc-exp a symlink or absent) before this test starts")
	require.Eventually(s.T(), s.stableInstallerDaemonLoaded, 60*time.Second, 2*time.Second,
		"host must be resting (the stable installer daemon running, its experiment job gone) before this test starts")
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
	// The command hands the installer daemon back from a detached process, after it exits.
	if !assert.Eventually(s.T(), s.stableInstallerDaemonLoaded, 60*time.Second, 2*time.Second) {
		s.T().Log("the stable installer daemon did not come back after remove-config-experiment")
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
	// First: dumpDiagnostics reads /status and restoreResting runs an installer command, and both
	// wait on an installer lock a failed test may have left held.
	s.releaseInstallerLock()
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

const (
	stableConfigBaselinePath = "/var/tmp/e2e-fleet-etc-baseline.tar"
	// configStateBaselinePath holds a copy of configStatePath taken with the stable configuration
	// baseline, or nothing when there was no state file then.
	configStateBaselinePath = "/var/tmp/e2e-fleet-config-state-baseline.json"
)

// saveStableConfigBaseline archives the stable configuration the suite starts from, so that
// resetStableConfig can put it back after every test.
//
// Without it, every promote leaves its change in stable configuration for the rest of the run, and
// each test runs against whatever the tests before it promoted.
func (s *configMacOSSuite) saveStableConfigBaseline() {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo tar --acls --xattrs -cpf %[1]s -C /opt/datadog-agent etc && sudo chmod 600 %[1]s", stableConfigBaselinePath))
	require.NoError(s.T(), err, "could not archive the stable configuration")
	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo sh -c 'rm -f %[2]s; if [ -f %[1]s ]; then cp -p %[1]s %[2]s; fi'", configStatePath, configStateBaselinePath))
	require.NoError(s.T(), err, "could not save the configuration state")
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
// The daemon reads its stable config version from the configuration state file, which is put back
// along with etc, so the daemon is restarted to report it. Its pending UPDATER_TASKs are
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
	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo sh -c 'if [ -f %[2]s ]; then cp -p %[2]s %[1]s; else rm -f %[1]s; fi'", configStatePath, configStateBaselinePath))
	require.NoError(s.T(), err, "could not restore the configuration state")
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

// swappableJobLabels are the agent jobs, which an experiment swaps synchronously. The installer
// daemon is swapped too, but asynchronously (see stableInstallerDaemonLoaded), so tests assert on it
// separately.
var swappableJobLabels = []string{"com.datadoghq.agent", "com.datadoghq.sysprobe", "com.datadoghq.data-plane"}

const installerDaemonLabel = "com.datadoghq.installer"

const installerDaemonExperimentLabel = installerDaemonLabel + "-exp"

// stableInstallerDaemonLoaded reports whether the installer daemon is back on its stable job, with
// its experiment job gone.
//
// The installer daemon is swapped asynchronously, by a detached process, once the installer
// command that started, stopped or promoted the experiment has exited. So this is polled: the
// daemon's state can report the outcome before the daemon itself has been swapped.
func (s *configMacOSSuite) stableInstallerDaemonLoaded() bool {
	loaded := s.launchdLabelsLoaded(installerDaemonLabel, installerDaemonExperimentLabel)
	return loaded[installerDaemonLabel] && !loaded[installerDaemonExperimentLabel]
}

// requireInstallerDaemon waits for the installer daemon to run on the given job, and the other one
// to be gone, then for it to answer on its socket.
func (s *configMacOSSuite) requireInstallerDaemon(running, gone string) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded(running, gone)
		assert.True(c, loaded[running], "%s should be loaded", running)
		assert.False(c, loaded[gone], "%s should no longer be loaded", gone)
	}, 60*time.Second, 2*time.Second)
	s.waitForDaemon()
}

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
	// installer-exp runs on etc-exp, whose config_id is the deployment.
	require.True(s.T(), s.waitForPackageState(func(pkg backend.RemoteConfigStatePackage) bool {
		return pkg.RunningConfigVersion == deploymentID
	}, 60*time.Second), "the experiment daemon should report the experiment as the running configuration")

	s.promoteConfigExperimentRC()
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
	require.True(s.T(), s.etcExpResting(), "etc-exp should be a symlink again after promotion")
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
	// The stable daemon now runs on the promoted configuration.
	require.True(s.T(), s.waitForPackageState(func(pkg backend.RemoteConfigStatePackage) bool {
		return pkg.RunningConfigVersion == deploymentID
	}, 60*time.Second), "the stable daemon should report the promoted configuration as running")
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
	require.True(s.T(), s.waitForPackageState(func(pkg backend.RemoteConfigStatePackage) bool {
		return pkg.RunningConfigVersion != deploymentID
	}, 60*time.Second), "the stable daemon should report the stable configuration as running after the stop")
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

// TestConfigFailureCrashMacOS pins that an experiment whose Agent refuses to start is put back on
// the stable jobs at once, and rolled back by Fleet's stop, with the bad value never reaching the
// stable configuration.
//
// The experiment sets log_level to an unresolvable ENC[...] placeholder, which agent-exp rejects at
// startup ("unknown log level"). The start itself succeeds, and the watcher
// (config_experiment_watcher_darwin.go) reverts every job on the exit. It keeps the experiment
// deployed and reported, with the stable daemon's config_id as the running configuration, so
// Fleet's health check fails and Fleet sends a stop; this test sends that stop itself.
// TestAgentExpCrashRevertsAndIsNotRelaunchedMacOS covers a crash of an Agent that did start.
func (s *configMacOSSuite) TestConfigFailureCrashMacOS() {
	s.requireResting()

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	stableFingerprint := s.stableConfigFingerprint()
	rejectionsBefore := s.agentExpLogLevelRejections()

	deploymentID := nextID("cfg-crash")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "ENC[invalid_secret]"}`)},
	}, nil)
	s.requireFailedOnHost(deploymentID)
	// Agent Data Plane reads the same log level and may exit first, before agent-exp logs its own
	// refusal, so the count only corroborates.
	s.T().Logf("agent-exp log level refusals: %d before, %d after", rejectionsBefore, s.agentExpLogLevelRejections())

	s.stopFailedExperimentRC()
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
// files: _dd-agent/admin mode 0660, the ownership the installer's .dmg set-up gives the
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

	// The installer daemon is part of the experiment too, reading the experiment configuration.
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	out, err = s.Env().RemoteHost.Execute("sudo launchctl print system/" + installerDaemonExperimentLabel)
	require.NoError(s.T(), err)
	require.Contains(s.T(), out, "/opt/datadog-agent/etc-exp", "the experiment daemon must read the experiment configuration")
	require.NotContains(s.T(), out, "KeepAlive", "the experiment daemon must not carry KeepAlive")

	s.stopConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)

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
// this test -- drives the revert. It puts every job back on stable, the installer daemon included,
// and keeps the experiment for Fleet's stop.
func (s *configMacOSSuite) TestWatcherRevertsOnSysprobeCrashMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-watcher-crash")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config": {"enabled": true}}`)},
	}, nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		loaded := s.launchdLabelsLoaded("com.datadoghq.sysprobe-exp")
		assert.True(c, loaded["com.datadoghq.sysprobe-exp"], "sysprobe experiment job should be loaded")
	}, 60*time.Second, 5*time.Second)

	_, err := s.Env().RemoteHost.Execute("sudo launchctl kill SIGKILL system/com.datadoghq.sysprobe-exp")
	require.NoError(s.T(), err, "sending SIGKILL to the experiment sysprobe job should succeed")

	// No task is sent until the revert is done: it must come from the watcher's own exit observer
	// noticing the abnormal exit. The stop afterwards is the one Fleet sends once its health check
	// fails.
	s.requireFailedOnHost(deploymentID)
	loadedStable := s.launchdLabelsLoaded("com.datadoghq.sysprobe")
	require.True(s.T(), loadedStable["com.datadoghq.sysprobe"], "sysprobe stable job should be loaded again after the watcher's revert")
	s.waitForDaemon()
	s.stopFailedExperimentRC()

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
// would otherwise never come back, are killed. The installer daemon itself, in both variants, is
// booted out rather than sent a plain kill, so that the stable job's KeepAlive stanza
// (RunAtLoad+KeepAlive.SuccessfulExit: false in com.datadoghq.installer.plist.tmpl) cannot race
// this helper by respawning it before the caller is ready -- bringing the daemon back up is always
// a separate, explicit restartInstallerDaemon call, which starts the stable daemon as a reboot
// would.
//
// Only the stable job set is restarted here, mirroring what launchd starts at boot: the experiment
// job set and the watcher stay down, and the experiment is left for Fleet's stop to end.
func (s *configMacOSSuite) killAllServicesSimulatingShutdown() {
	// Every detached process dies with the host, not only one watcher: another one still around
	// would otherwise revert the experiment under the test.
	_, _ = s.Env().RemoteHost.Execute("sudo pkill -SIGKILL -f '" + detachedProcessPattern + "' || true")

	_, err := s.Env().RemoteHost.Execute("sudo launchctl bootout system/com.datadoghq.installer || true")
	require.NoError(s.T(), err, "booting out the installer daemon should succeed")
	_, err = s.Env().RemoteHost.Execute("sudo launchctl bootout system/" + installerDaemonExperimentLabel + " || true")
	require.NoError(s.T(), err, "booting out the experiment installer daemon should succeed")

	for _, label := range append(append([]string{}, swappableJobLabels...), experimentLabels()...) {
		_, _ = s.Env().RemoteHost.Execute("sudo launchctl kill SIGKILL system/" + label + " || true")
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("pgrep -f '" + detachedProcessPattern + "' || true")
		assert.NoError(c, err)
		assert.Empty(c, strings.TrimSpace(out), "the detached installer processes should be gone")
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

// processElapsed returns how long the process pid has been running, as ps reports it.
func (s *configMacOSSuite) processElapsed(pid string) time.Duration {
	out, err := s.Env().RemoteHost.Execute("ps -o etime= -p " + pid)
	require.NoError(s.T(), err, "pid %s should be running", pid)
	return parseElapsed(strings.TrimSpace(out))
}

// parseElapsed parses the [[dd-]hh:]mm:ss form ps prints an elapsed time in.
func parseElapsed(etime string) time.Duration {
	days := 0
	if d, rest, found := strings.Cut(etime, "-"); found {
		days, _ = strconv.Atoi(d)
		etime = rest
	}
	seconds := 0
	for _, part := range strings.Split(etime, ":") {
		n, _ := strconv.Atoi(part)
		seconds = seconds*60 + n
	}
	return time.Duration(days*86400+seconds) * time.Second
}

// detachedProcessPattern matches every detached process a configuration experiment runs outside of
// any launchd job: the watchers that supervise the experiment, the launcher of the second one, and
// the fallback hand-back.
const detachedProcessPattern = "package-command datadog-agent (watchConfigExperiment|startSupervisorRelay|handBackInstaller)"

// requireWatcherRunning waits for a watcher to supervise the experiment. The start hook launches the
// first one; once the installer daemon is handed over, the experiment daemon launches the second,
// which takes over.
func (s *configMacOSSuite) requireWatcherRunning(why string) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.watcherRunning(), "a watcher should supervise the experiment: %s", why)
	}, 60*time.Second, 2*time.Second)
}

// watcherPID returns the pid of the experiment watcher, or "" when none is running.
func (s *configMacOSSuite) watcherPID() string {
	out, err := s.Env().RemoteHost.Execute("pgrep -f 'package-command datadog-agent watchConfigExperiment' | head -n 1 || true")
	require.NoError(s.T(), err)
	return strings.TrimSpace(out)
}

// TestShutdownMidExperimentWaitsForFleetMacOS covers an abrupt shutdown mid-experiment, which leaves
// the deadline in place and no watcher, with the deadline still valid or already expired by the
// next start: nothing at start brings the experiment back or settles it. The host comes back on the
// stable job set and reports the experiment for Fleet to stop, and the stop ends it without
// restarting the stable Agent.
func (s *configMacOSSuite) TestShutdownMidExperimentWaitsForFleetMacOS() {
	for _, expired := range []bool{false, true} {
		s.Run(fmt.Sprintf("expired=%t", expired), func() {
			s.requireResting()

			deploymentID := nextID("cfg-shutdown")
			s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
				{FileOperationType: backend.FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config": {"enabled": true}}`)},
			}, nil)
			s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
			s.requireWatcherRunning("the experiment is live")

			s.killAllServicesSimulatingShutdown()
			require.False(s.T(), s.watcherRunning(), "the watcher process must be gone before the daemon starts")
			if expired {
				_, err := s.Env().RemoteHost.Execute(
					`sudo sh -c 'date -u -v-2H +"%Y-%m-%dT%H:%M:%SZ" > ` + experimentDeadlinePath + `'`)
				require.NoError(s.T(), err, "forcing the experiment deadline into the past should succeed")
			}
			// See clearPendingUpdaterTasks: the start task pushed above would otherwise be redelivered
			// to the restarted daemon.
			s.clearPendingUpdaterTasks()
			s.restartInstallerDaemon()

			s.requireUnfinishedOnHost(deploymentID)
			s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
			s.stopUnfinishedExperimentRC()
		})
	}
}

// TestGracefulShutdownMidExperimentFailsForFleetMacOS covers what a graceful shutdown does to a live
// experiment: launchd sends SIGTERM to every process, the watcher keeps supervising for the grace
// its signal handler gives it, sees the essential experiment jobs exit, and fails the experiment
// before the host goes down. The stable daemon starting next finds the failed marker and leaves the
// host as it is, for Fleet to stop.
func (s *configMacOSSuite) TestGracefulShutdownMidExperimentFailsForFleetMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-graceful-shutdown")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	s.requireWatcherRunning("the experiment is live")
	s.clearPendingUpdaterTasks()

	_, err := s.Env().RemoteHost.Execute(
		"sudo pkill -TERM -f 'package-command datadog-agent watchConfigExperiment'; " +
			"sudo launchctl kill SIGTERM system/com.datadoghq.agent-exp")
	require.NoError(s.T(), err, "signalling the watcher and the experiment Agent should succeed")

	s.requireFailedOnHost(deploymentID)
	s.requireWatcherExits("failed on shutdown")

	// The stable daemon starting again, as on the next boot, leaves the failed experiment alone.
	agentPID := s.jobPID("com.datadoghq.agent")
	_, err = s.Env().RemoteHost.Execute("sudo launchctl kickstart -k system/" + installerDaemonLabel)
	require.NoError(s.T(), err, "restarting the stable installer daemon should succeed")
	s.waitForDaemon()
	s.requireFailedOnHost(deploymentID)
	require.Equal(s.T(), agentPID, s.jobPID("com.datadoghq.agent"), "a daemon start must not restart the stable Agent")
	require.Zero(s.T(), s.watcherCount(), "nothing should launch a watcher on a daemon start")
	s.stopFailedExperimentRC()
}

// TestRebootMidExperimentWaitsForFleetMacOS reboots the host in the middle of an experiment, with
// shutdown(8): the host boots on the stable job set and reports the experiment, and Fleet's stop
// ends it without restarting any job launchd started at boot. The experiment daemon's job, which
// launchd loads at boot without starting it, is unloaded by the stop.
//
// It is skipped on a LOCAL_VM host: the suite's fakeintake runs there as a container
// (byohost.LocalVMProvisioner) that does not come back after a reboot, and the Agent would lose the
// Remote Config server this suite drives it through.
func (s *configMacOSSuite) TestRebootMidExperimentWaitsForFleetMacOS() {
	if os.Getenv("LOCAL_VM") != "" {
		s.T().Skip("the LOCAL_VM fakeintake container does not survive a reboot of its host")
	}
	s.requireResting()

	deploymentID := nextID("cfg-reboot")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	s.requireWatcherRunning("the experiment is live")
	s.clearPendingUpdaterTasks()

	s.rebootHost()
	s.waitForDaemon()

	// On a graceful shutdown the watcher usually fails the experiment before the host goes down;
	// either way the host comes back on the stable job set, reporting the experiment.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			assert.Equal(c, deploymentID, status.Packages[0].ExperimentConfigVersion, "the experiment should still be reported, for Fleet to stop")
		}
	}, 90*time.Second, 5*time.Second)
	require.Equal(s.T(), "1", s.jobRuns("com.datadoghq.agent"), "nothing should restart the stable Agent launchd started at boot")
	require.Zero(s.T(), s.watcherCount(), "nothing should launch a watcher on a daemon start")
	s.stopUnfinishedExperimentRC()
	require.Equal(s.T(), "1", s.jobRuns("com.datadoghq.agent"), "the stop must not restart the stable Agent launchd started at boot")
}

// rebootHost reboots the host gracefully and waits until it is back on a new boot session.
// shutdown(8) can drop the ssh session before it returns, so its result is ignored: the boot
// session is what tells the host has rebooted.
func (s *configMacOSSuite) rebootHost() {
	host := s.Env().RemoteHost
	before := strings.TrimSpace(host.MustExecute("sysctl -n kern.bootsessionuuid"))
	if _, err := host.Execute("sudo shutdown -r now"); err != nil {
		s.T().Logf("reboot request: %v", err)
	}
	time.Sleep(20 * time.Second)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		if !assert.NoError(c, host.Reconnect()) {
			return
		}
		after, err := host.Execute("sysctl -n kern.bootsessionuuid")
		if assert.NoError(c, err) {
			assert.NotEqual(c, before, strings.TrimSpace(after), "the host should be on a new boot session")
		}
	}, 20*time.Minute, 15*time.Second)
}

// jobRuns returns how many times launchd has started the job since it was loaded, as launchctl
// print reports it.
func (s *configMacOSSuite) jobRuns(label string) string {
	out, err := s.Env().RemoteHost.Execute(
		"sudo launchctl print system/" + label + ` 2>/dev/null | awk '$1 == "runs" && $2 == "=" { print $3; exit }' || true`)
	require.NoError(s.T(), err)
	return strings.TrimSpace(out)
}

// --- Set 15: daemon socket, watcher lifecycle and upgrades (macOS-specific) --------------------

const (
	installerSocketPath    = "/opt/datadog-agent/run/installer.sock"
	experimentDeadlinePath = "/opt/datadog-packages/run/experiment-deadline"
	// configStatePath records which configuration is deployed: the stable and experiment deployment
	// IDs, and the inode of the experiment's directory.
	configStatePath = "/opt/datadog-packages/run/config-state.json"
	// installerPIDPath is the root installer daemon's pidfile.
	installerPIDPath = "/opt/datadog-packages/run/installer.pid"
)

// configState is the content of configStatePath.
type configState struct {
	StableDeploymentID string `json:"stable_deployment_id"`
	Experiment         *struct {
		DeploymentID string `json:"deployment_id"`
		Inode        uint64 `json:"inode"`
	} `json:"experiment"`
}

// readConfigState returns the configuration state file's content.
func (s *configMacOSSuite) readConfigState() configState {
	var state configState
	require.NoError(s.T(), json.Unmarshal([]byte(s.readFile(configStatePath)), &state), "could not parse %s", configStatePath)
	return state
}

// TestInstallerStateIsRootOwnedMacOS pins where the root installer keeps its own state: the
// configuration state file and the daemon's pidfile are root's, in the root-only state directory,
// and neither the configuration tree nor the Agent's run directory holds any of it.
func (s *configMacOSSuite) TestInstallerStateIsRootOwnedMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-state"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	// The start is reported before the installer daemon is handed over to its experiment job, and
	// the pidfile is briefly absent between the stable daemon exiting and the experiment daemon
	// writing it.
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	for _, path := range []string{configStatePath, installerPIDPath} {
		perms, err := s.Host.GetFilePermissions(path)
		require.NoError(s.T(), err, "%s should exist", path)
		require.Equal(s.T(), [3]string{"644", "root", "wheel"}, [3]string{perms.Mode, perms.Owner, perms.Group},
			"%s should be root:wheel 0644", path)
	}
	for _, path := range []string{
		"/opt/datadog-agent/run/installer.pid",
		"/opt/datadog-agent/etc/.deployment-id",
		"/opt/datadog-agent/etc-exp/.deployment-id",
		"/opt/datadog-agent/etc-exp/.promoting",
	} {
		require.False(s.T(), s.pathExists(path), "%s should not exist", path)
	}
	s.stopConfigExperimentRC()
}

// socketRequestAs calls the installer daemon's local API as user and returns the HTTP status code it
// answered with.
func (s *configMacOSSuite) socketRequestAs(user, method, path string) string {
	out, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`sudo -u %s curl -sS -o /dev/null -w '%%{http_code}' -X %s -H 'Content-Type: application/json' --unix-socket %s http://installer%s`,
		user, method, installerSocketPath, path))
	require.NoError(s.T(), err, "%s should be able to connect to the installer socket", user)
	return strings.TrimSpace(out)
}

// TestInstallerSocketIsOwnerOnlyMacOS pins pkg/fleet/daemon/local_api_unix.go on macOS: the
// daemon runs as root and hands its socket to _dd-agent, mode 0700, so the Agent's account reaches
// the local API as it does on Linux, and no other account can connect.
func (s *configMacOSSuite) TestInstallerSocketIsOwnerOnlyMacOS() {
	s.requireResting()

	// The configuration experiment state root acts on is kept in a directory that is root's
	// alone, unlike the Agent's run directory the socket is in.
	stateDir, err := s.Host.GetFilePermissions(filepath.Dir(experimentDeadlinePath))
	require.NoError(s.T(), err)
	require.Equal(s.T(), [3]string{"755", "root", "wheel"}, [3]string{stateDir.Mode, stateDir.Owner, stateDir.Group},
		"the configuration experiment state directory should be root:wheel 0755")

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
	require.Equal(s.T(), "200", s.socketRequestAs("_dd-agent", "GET", "/status"),
		"the Agent's account should be able to read the daemon's status")
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

// requireSingleSupervisor waits for the relay to settle: the watcher the experiment daemon launched
// (W2) has stopped the one the start hook launched (W1), which keeps supervising for its signal
// grace after SIGTERM, so up to about 15 seconds after W2 starts. It returns W2's pid.
func (s *configMacOSSuite) requireSingleSupervisor() string {
	var watcher string
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Equal(c, 1, s.watcherCount(), "exactly one watcher should supervise once the relay is done")
		watcher = s.watcherPID()
	}, 60*time.Second, 2*time.Second)
	require.LessOrEqual(s.T(), s.processElapsed(watcher), s.processElapsed(s.jobPID(installerDaemonExperimentLabel)),
		"the watcher left supervising should be the one the experiment daemon launched")
	return watcher
}

// TestSecondWatcherTakesSupervisionOverMacOS pins the relay end to end: once the experiment runs,
// one watcher is left, younger than the experiment daemon that launched it; the stop then ends the
// experiment and the watcher with it.
func (s *configMacOSSuite) TestSecondWatcherTakesSupervisionOverMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-relay"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	s.requireSingleSupervisor()

	s.stopConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
	s.requireWatcherExits("stopped")
}

// TestStopWithoutASupervisorFallsBackToTheHandBackMacOS covers the supervising watcher dying after
// the relay: the experiment then runs unsupervised until Fleet stops it, and the stop, finding no
// watcher running, launches the hand-back process to return the installer daemon.
func (s *configMacOSSuite) TestStopWithoutASupervisorFallsBackToTheHandBackMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-no-supervisor"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	watcher := s.requireSingleSupervisor()
	_, err := s.Env().RemoteHost.Execute("sudo kill -9 " + watcher)
	require.NoError(s.T(), err)
	s.requireWatcherExits("killed")
	require.False(s.T(), s.etcExpResting(), "killing the watcher must not end the experiment")

	s.stopConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
	require.True(s.T(), s.etcExpResting(), "the stop should discard the experiment")
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "the stop should clear the deadline")
}

// TestStopDuringTheWatcherOverlapMacOS covers a stop that reaches the experiment daemon while both
// watchers still run, W1 within its signal grace: only W2 hands the installer daemon back, and the
// host ends resting with the stable daemon back.
func (s *configMacOSSuite) TestStopDuringTheWatcherOverlapMacOS() {
	s.requireResting()

	s.startConfigExperimentRC(nextID("cfg-overlap-stop"), []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	s.T().Logf("watchers running when the stop is pushed: %d", s.watcherCount())

	s.stopConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
	require.True(s.T(), s.etcExpResting(), "the stop should discard the experiment")
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "the stop should clear the deadline")
	s.requireWatcherExits("stopped during the overlap")
}

// TestAgentCrashDuringTheWatcherOverlapMacOS covers an agent experiment job failing while both
// watchers run: W2 owns the failure, and the experiment ends failed for Fleet to stop, the same as
// with a single watcher. Only the outcome is checked: the detached watchers keep no log.
func (s *configMacOSSuite) TestAgentCrashDuringTheWatcherOverlapMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-overlap-crash")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	// Land the crash while both watchers run if possible; the outcome is the same either way.
	overlap := false
	for i := 0; i < 20 && !overlap; i++ {
		overlap = s.watcherCount() == 2
		if !overlap {
			time.Sleep(250 * time.Millisecond)
		}
	}
	s.T().Logf("both watchers running when agent-exp is killed: %t", overlap)
	s.clearPendingUpdaterTasks()
	_, err := s.Env().RemoteHost.Execute("sudo launchctl kill SIGKILL system/com.datadoghq.agent-exp")
	require.NoError(s.T(), err)

	s.requireFailedOnHost(deploymentID)
	s.requireWatcherExits("reverted the crashed experiment")
	s.stopFailedExperimentRC()
}

// TestStopWhoseCommandDiesIsFinishedByTheWatcherMacOS covers the mid-stop crash: the experiment
// daemon, and the stop command it runs, die once the stop has marked the deadline as reverting.
// Where the kill lands decides what is left, and both are ends Fleet's next stop cleans up:
//
//   - inside the restore, with the jobs half handed back: the watcher sees the daemon go, waits for
//     the lock the dead command held, and finishes the restore, every job and the daemon with it,
//     marking the experiment failed;
//   - after the hook handed the Agent back and cleared the deadline, before the installer discarded
//     the configuration: the watcher hands the daemon back, and the experiment is left deployed
//     with no deadline.
//
// Either way every job ends on its stable variant, the stable daemon included, the experiment is
// still reported, and nothing is left half switched.
func (s *configMacOSSuite) TestStopWhoseCommandDiesIsFinishedByTheWatcherMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-mid-stop-crash")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	s.requireSingleSupervisor()

	// Kill the experiment daemon and its stop command as soon as the stop marks the deadline as
	// reverting, from a background shell on the host, so the kill lands inside the stop.
	_, err := s.Env().RemoteHost.Execute(`sudo sh -c 'nohup sh -c "` +
		`for i in \$(seq 1 6000); do [ \"\$(cat ` + experimentDeadlinePath + ` 2>/dev/null)\" = reverting ] && break; sleep 0.01; done; ` +
		`pkill -9 -f \"installer remove-config-experiment\"; launchctl kill SIGKILL system/` + installerDaemonExperimentLabel +
		`" >/dev/null 2>&1 &'`)
	require.NoError(s.T(), err)
	s.pushTask("stop_experiment_config", s.currentExpectedState(), nil)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s should be unloaded", label)
		}
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running")
		assert.True(c, s.stableInstallerDaemonLoaded(), "the stable installer daemon should be back")
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			assert.Equal(c, deploymentID, status.Packages[0].ExperimentConfigVersion, "the experiment should still be reported, for Fleet to stop")
		}
	}, 90*time.Second, 5*time.Second)
	require.False(s.T(), s.etcExpResting(), "the experiment's configuration should be kept for Fleet's stop")
	deadline := ""
	if s.pathExists(experimentDeadlinePath) {
		deadline = strings.TrimSpace(s.readFile(experimentDeadlinePath))
	}
	require.Contains(s.T(), []string{"failed", ""}, deadline, "the deadline should be failed, or cleared by the hook before the kill")
	s.T().Logf("the kill landed %s", map[string]string{"failed": "inside the restore", "": "after the hook cleared the deadline"}[deadline])
	s.requireWatcherExits("finished the interrupted stop")

	// See clearPendingUpdaterTasks: the killed stop is never reported as done.
	s.clearPendingUpdaterTasks()
	agentPID := s.jobPID("com.datadoghq.agent")
	s.stopConfigExperimentRC()
	require.True(s.T(), s.etcExpResting(), "the stop should discard the experiment's configuration")
	require.False(s.T(), s.pathExists(experimentDeadlinePath), "the stop should leave no deadline")
	require.Equal(s.T(), agentPID, s.jobPID("com.datadoghq.agent"), "the stop must not restart the stable Agent")
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

// TestUpgradeMidExperimentClearsDeadlineMacOS regression-tests the .dmg install over a live
// configuration experiment: the installed version's pre-remove takes the -exp jobs down, and
// `installer postinst datadog-agent dmg` removes the experiment's deadline (removeOldInstallFiles)
// and collapses etc-exp back onto etc (restExperimentConfig). A deadline left behind would outlive
// the experiment the upgrade discarded.
//
// The upgrade is a reinstall of the same .dmg over the running host, which is what runs the
// preinstall script's existing-installation path. It is the slowest test in the suite for that
// reason.
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
			assert.False(c, loaded, "%s must not be loaded after the upgrade", label)
		}
		loaded := s.launchdLabelsLoaded("com.datadoghq.agent")
		assert.True(c, loaded["com.datadoghq.agent"], "the stable Agent job should be running after the upgrade")
		assert.True(c, s.stableInstallerDaemonLoaded(), "only the stable installer daemon should run after the upgrade")
	}, 60*time.Second, 5*time.Second)
	s.requireWatcherExits("discarded by the upgrade")
}

// --- Set 16: on-disk and launchd invariants of the experiment lifecycle (macOS-specific) --------

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
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)

	allJobLabels := append(append([]string{}, swappableJobLabels...), installerDaemonLabel)
	for _, label := range allJobLabels {
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

		experimentArgs, ok := experiment["ProgramArguments"].([]any)
		require.True(s.T(), ok, "%s should have ProgramArguments", path)
		if label == installerDaemonLabel {
			// The experiment daemon launches the second watcher, then becomes the daemon its stable
			// twin runs, on etc-exp.
			require.Len(s.T(), experimentArgs, 3, "%s should run through a shell", path)
			require.Equal(s.T(), "/bin/sh", experimentArgs[0], "%s should run through a shell", path)
			script, _ := experimentArgs[2].(string)
			require.Contains(s.T(), script, "package-command datadog-agent startSupervisorRelay;", "%s should launch the second watcher first", path)
			require.Contains(s.T(), script, "exec '/opt/datadog-agent/embedded/bin/installer' run -c '/opt/datadog-agent/etc-exp'", "%s should then become the daemon on etc-exp", path)
		} else {
			// Mapping etc-exp back to etc must give exactly the stable job's command line: the same
			// program, with only the configuration directory swapped.
			var mapped []any
			var readsExperiment bool
			for _, arg := range experimentArgs {
				str, _ := arg.(string)
				readsExperiment = readsExperiment || strings.Contains(str, "/opt/datadog-agent/etc-exp")
				mapped = append(mapped, strings.ReplaceAll(str, "/opt/datadog-agent/etc-exp", "/opt/datadog-agent/etc"))
			}
			require.True(s.T(), readsExperiment, "%s should read its configuration from etc-exp", path)
			require.Equal(s.T(), stable["ProgramArguments"], mapped, "%s should run its stable twin's program on etc-exp", path)
		}

		env, _ := experiment["EnvironmentVariables"].(map[string]any)
		require.NotContains(s.T(), env, "DD_FLEET_POLICIES_DIR", "%s must not point policies outside the configuration directory", path)
		if label == "com.datadoghq.agent" {
			require.Equal(s.T(), "/opt/datadog-agent/etc-exp/conf.d", env["DD_CONFD_PATH"], "%s should load checks from etc-exp", path)
		}
		require.NotContains(s.T(), env, "DD_INSTALLER_EXPERIMENT_DAEMON", "%s has no use for the experiment-daemon marker", path)
	}

	s.stopConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)

	for _, label := range allJobLabels {
		require.False(s.T(), s.pathExists(experimentPlistPath(label)), "%s should be removed once the experiment ends", experimentPlistPath(label))
		_, err := s.Env().RemoteHost.Execute("sudo launchctl print system/" + label + "-exp")
		require.Error(s.T(), err, "%s-exp should be gone from launchd once the experiment ends", label)
	}
}

// TestAgentExpCrashRevertsAndIsNotRelaunchedMacOS covers the first failure mode, "the -exp Agent
// crashes": launchd does not relaunch it, since the experiment set has no KeepAlive, and the
// watcher (config_experiment_watcher_darwin.go) puts every job back on stable on its exit, the
// installer daemon included, and keeps the experiment for Fleet's stop.
// TestWatcherRevertsOnSysprobeCrashMacOS covers the same path for system-probe.
func (s *configMacOSSuite) TestAgentExpCrashRevertsAndIsNotRelaunchedMacOS() {
	s.requireResting()

	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)

	deploymentID := nextID("cfg-agent-crash")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
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

	s.requireFailedOnHost(deploymentID)
	require.False(s.T(), s.pathExists(experimentPlistPath("com.datadoghq.agent")), "the revert should remove the experiment job definitions")
	s.waitForDaemon()

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), before["log_level"], config["log_level"], "the stable Agent should read the stable configuration")
	s.stopFailedExperimentRC()
}

// TestDeadlineFileLifecycleMacOS pins the persisted deadline (pkg/fleet/installer/packages/launchd/
// supervisor.go, config_experiment_darwin.go): it exists only while an experiment is live, belongs
// to root, and holds a time configExperimentDeadlineWindow (50 minutes) after the start. It is what
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
		require.WithinRange(s.T(), deadline, started.Add(49*time.Minute), s.remoteNow().Add(51*time.Minute),
			"the deadline should be 50 minutes after the start")

		end[how]()
		require.False(s.T(), s.pathExists(experimentDeadlinePath), "the deadline should be cleared once the experiment is %s", how)
	}
}

// TestExperimentDaemonCrashRevertsMacOS covers the installer daemon dying in the middle of an
// experiment. The daemon runs on its experiment job then, which launchd does not relaunch, so the
// watcher treats its exit like any other experiment job's crash: it reverts every job, the daemon
// included, and keeps the experiment for Fleet's stop. A stable daemon start with an experiment
// left unfinished is covered by TestShutdownMidExperimentFailsForFleetMacOS.
func (s *configMacOSSuite) TestExperimentDaemonCrashRevertsMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-daemon-crash")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Equal(c, 1, s.watcherCount(), "exactly one watcher should supervise the live experiment")
	}, 30*time.Second, 2*time.Second)
	pid := s.jobPID(installerDaemonExperimentLabel)
	require.NotEmpty(s.T(), pid, "the experiment daemon should be running")

	// See clearPendingUpdaterTasks: a restarted daemon has no memory of the tasks it executed.
	s.clearPendingUpdaterTasks()
	_, err := s.Env().RemoteHost.Execute("sudo kill -9 " + pid)
	require.NoError(s.T(), err)

	s.requireFailedOnHost(deploymentID)
	s.waitForDaemon()
	s.requireWatcherExits("reverted after the experiment daemon crashed")
	s.stopFailedExperimentRC()
}

// TestOrphanExperimentWithoutDeadlineIsStoppedByFleetMacOS covers an experiment still deployed in
// etc-exp with no deadline left to bound it -- a stop interrupted between clearing the deadline and
// discarding the configuration. Removing the deadline tells the watcher a deliberate end has begun,
// so it exits without reverting and leaves exactly that orphan behind. Nothing at daemon start
// discards it: the host goes on reporting the experiment, with the stable configuration running,
// and Fleet's stop reverts and discards it.
func (s *configMacOSSuite) TestOrphanExperimentWithoutDeadlineIsStoppedByFleetMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-orphan")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	// The deadline is only removed once the experiment is under way -- the daemon handed over and
	// its watcher running. Removing it earlier is an experiment that ended while the daemon was
	// being handed over, which is handed straight back and so not an orphan.
	s.requireWatcherRunning("before the deadline is removed")
	_, err := s.Env().RemoteHost.Execute("sudo rm " + experimentDeadlinePath)
	require.NoError(s.T(), err)
	s.requireWatcherExits("left without a deadline")
	require.False(s.T(), s.etcExpResting(), "the experiment should still be deployed once its watcher has exited")

	// The stable daemon starts as after a reboot. The experiment daemon is booted out first: with
	// its watcher gone, nothing reverts on its exit.
	s.clearPendingUpdaterTasks()
	_, err = s.Env().RemoteHost.Execute("sudo launchctl bootout system/" + installerDaemonExperimentLabel)
	require.NoError(s.T(), err)
	s.restartInstallerDaemon()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if assert.NoError(c, err) && assert.Len(c, status.Packages, 1) {
			pkg := status.Packages[0]
			assert.Equal(c, deploymentID, pkg.ExperimentConfigVersion, "the orphan should still be reported, for Fleet to stop")
			assert.NotEqual(c, deploymentID, pkg.RunningConfigVersion, "the stable daemon should report the stable configuration as running")
		}
	}, 60*time.Second, 5*time.Second)
	require.False(s.T(), s.etcExpResting(), "nothing at daemon start should discard the orphan")
	require.Zero(s.T(), s.watcherCount(), "nothing should launch a watcher on a daemon start")

	s.stopConfigExperimentRC()
	require.True(s.T(), s.etcExpResting(), "the stop should discard the orphan")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for label, loaded := range s.launchdLabelsLoaded(experimentLabels()...) {
			assert.False(c, loaded, "%s should be unloaded by the stop", label)
		}
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running again")
	}, 60*time.Second, 5*time.Second)
}

// TestInstallerDaemonSwappedWithTheExperimentMacOS pins that the installer daemon is part of a
// configuration experiment: it runs on its experiment job, reading etc-exp, for as long as the
// experiment does, and is back on its stable job once the experiment is promoted or stopped.
func (s *configMacOSSuite) TestInstallerDaemonSwappedWithTheExperimentMacOS() {
	s.requireResting()

	patch := []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}
	stablePID := s.jobPID(installerDaemonLabel)
	require.NotEmpty(s.T(), stablePID, "the installer daemon should be running")

	deploymentID := nextID("cfg-daemon-swap-promote")
	s.startConfigExperimentRC(deploymentID, patch, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	require.Equal(s.T(), deploymentID, s.packageState(s.readStatus()).ExperimentConfigVersion,
		"the experiment daemon should report the experiment")
	// The experiment daemon launches the watcher left supervising, so it is no older than that
	// daemon: the first watcher, launched before the swap, would be older.
	s.requireSingleSupervisor()

	s.promoteConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
	require.NotEqual(s.T(), stablePID, s.jobPID(installerDaemonLabel), "the stable daemon should have been restarted")
	require.False(s.T(), s.pathExists(experimentPlistPath(installerDaemonLabel)), "the experiment daemon's definition should be removed")

	s.startConfigExperimentRC(nextID("cfg-daemon-swap-stop"), patch, nil)
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	s.stopConfigExperimentRC()
	s.requireInstallerDaemon(installerDaemonLabel, installerDaemonExperimentLabel)
	require.False(s.T(), s.pathExists(experimentPlistPath(installerDaemonLabel)), "the experiment daemon's definition should be removed")
}

// TestPromoteSwapsTheExperimentDirectoryMacOS pins promote and stop at the directory level
// (pkg/fleet/installer/config/config_darwin.go, dir_swap.go): promote moves the experiment
// directory itself onto etc rather than copying it, and stop leaves etc
// untouched. It does not tell the atomic swap apart from a pair of renames; the unit tests cover
// that.
func (s *configMacOSSuite) TestPromoteSwapsTheExperimentDirectoryMacOS() {
	s.requireResting()

	deploymentID := nextID("cfg-swap")
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	experimentInode := s.inode("/opt/datadog-agent/etc-exp")
	state := s.readConfigState()
	require.NotNil(s.T(), state.Experiment, "the configuration state should record the experiment")
	require.Equal(s.T(), deploymentID, state.Experiment.DeploymentID, "the configuration state should record the deployment etc-exp holds")
	require.Equal(s.T(), experimentInode, strconv.FormatUint(state.Experiment.Inode, 10),
		"the configuration state should record etc-exp's directory")

	s.promoteConfigExperimentRC()
	require.Equal(s.T(), experimentInode, s.inode("/opt/datadog-agent/etc"), "promote should move the experiment directory onto etc")
	state = s.readConfigState()
	require.Equal(s.T(), deploymentID, state.StableDeploymentID, "the configuration state should record the promoted deployment")
	require.Nil(s.T(), state.Experiment, "the configuration state should record no experiment once it is promoted")
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
// the files a Fleet operation writes take the installer's .dmg set-up _dd-agent:admin 0660.
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
			`find . -mindepth 1 ! -path ./datadog.yaml | sort | while read -r f; do `+
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

// --- Set 17: watcher revert serialized with installer commands (macOS-specific) -----------------

const (
	// installerLockPath is the installer's package database. Every installer command holds an
	// exclusive flock(2) on it for its whole run, and the watcher takes it before reverting
	// (revertFromWatcher in pkg/fleet/installer/packages/config_experiment_watcher_darwin.go).
	installerLockPath = "/opt/datadog-packages/packages.db"
	// installerLockHolderPID and installerLockHeldMarker belong to holdInstallerLock's stand-in
	// for a running installer command.
	installerLockHolderPID  = "/tmp/dd-e2e-installer-lock.pid"
	installerLockHeldMarker = "/tmp/dd-e2e-installer-lock.held"
)

// holdInstallerLock stands in for a long-running installer command: it takes the same exclusive
// flock(2) bbolt takes on the package database (go.etcd.io/bbolt's flock, LOCK_EX), from a
// background process, and returns once the lock is held. The returned function releases it.
//
// Callers defer the release, so it runs before AfterTest: AfterTest's diagnostics and rollback wait
// on the lock too. AfterTest also releases it, in case a test ends without its defer running.
//
// While it is held, every installer command and the daemon's state refresh wait on it -- including
// the state check the daemon runs before executing a Remote Config task -- so a test must neither
// read the daemon's /status nor push a task until it has released it.
func (s *configMacOSSuite) holdInstallerLock() (release func()) {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`sudo rm -f %[2]s && sudo sh -c '/usr/bin/perl -e '"'"'use Fcntl qw(:flock); `+
			`open(my $f, "<", "%[1]s") or die $!; flock($f, LOCK_EX) or die $!; `+
			`open(my $m, ">", "%[2]s") or die $!; close $m; sleep 600;'"'"' </dev/null >/dev/null 2>&1 & echo $! > %[3]s'`,
		installerLockPath, installerLockHeldMarker, installerLockHolderPID))
	require.NoError(s.T(), err, "starting the installer lock holder should succeed")
	require.Eventually(s.T(), func() bool { return s.pathExists(installerLockHeldMarker) },
		30*time.Second, 500*time.Millisecond, "the lock holder should take the installer lock")
	return s.releaseInstallerLock
}

// releaseInstallerLock stops holdInstallerLock's lock holder, if one is running. Idempotent.
func (s *configMacOSSuite) releaseInstallerLock() {
	out, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`sudo sh -c 'if [ -f %[1]s ]; then kill $(cat %[1]s) 2>/dev/null; fi; rm -f %[1]s %[2]s'`,
		installerLockHolderPID, installerLockHeldMarker))
	if err != nil {
		s.T().Logf("could not release the installer lock: %v (%s)", err, out)
	}
}

// startExperimentAndCrashIt deploys a configuration experiment that sets log_level to debug, holds
// the installer lock, and kills the experiment Agent, so the watcher is left wanting to revert
// while an installer command is "running". It returns the deployment ID, the expected state from
// before the lock was taken, and the lock's release function, which the caller defers.
func (s *configMacOSSuite) startExperimentAndCrashIt(prefix string) (string, expectedState, func()) {
	deploymentID := nextID(prefix)
	s.startConfigExperimentRC(deploymentID, []backend.FileOperation{
		{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
	}, nil)
	var pid string
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		pid = s.jobPID("com.datadoghq.agent-exp")
		assert.NotEmpty(c, pid, "the experiment Agent should be running")
	}, 30*time.Second, 2*time.Second)
	// The first watcher hands the installer daemon over once the start command exits, and the second
	// takes supervision over, so both must be done before the lock is taken: the revert is what this
	// sets up to wait on, not the relay.
	s.requireInstallerDaemon(installerDaemonExperimentLabel, installerDaemonLabel)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.watcherRunning(), "a watcher should supervise the experiment")
	}, 30*time.Second, time.Second)
	s.requireSingleSupervisor()
	// Read before the lock is taken: the daemon's /status waits on it.
	expected := s.currentExpectedState()

	release := s.holdInstallerLock()
	_, err := s.Env().RemoteHost.Execute("sudo kill -9 " + pid)
	require.NoError(s.T(), err)
	return deploymentID, expected, release
}

// requireExperimentStillDeployed requires, for the whole of window, that the experiment the
// watcher wants to revert is left in place: its deadline, its directory and its job definitions.
func (s *configMacOSSuite) requireExperimentStillDeployed(window time.Duration) {
	for end := time.Now().Add(window); time.Now().Before(end); time.Sleep(3 * time.Second) {
		require.True(s.T(), s.pathExists(experimentDeadlinePath), "the watcher reverted while an installer command held the lock")
		require.False(s.T(), s.etcExpResting(), "the watcher discarded the experiment while an installer command held the lock")
		require.True(s.T(), s.pathExists(experimentPlistPath("com.datadoghq.agent")), "the watcher removed the experiment jobs while an installer command held the lock")
	}
}

// requireConfigurationIntact requires the end state every serialized outcome shares: nothing
// deployed, the stable Agent running, and a stable configuration that is whole and is the one
// wantLogLevel names.
func (s *configMacOSSuite) requireConfigurationIntact(wantLogLevel any) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.True(c, s.etcExpResting(), "etc-exp should rest on etc again")
		assert.False(c, s.pathExists(experimentDeadlinePath), "the deadline should be cleared")
		assert.False(c, s.pathExists(experimentPlistPath("com.datadoghq.agent")), "the experiment job definitions should be removed")
		assert.NotEmpty(c, s.jobPID("com.datadoghq.agent"), "the stable Agent should be running")
	}, 2*time.Minute, 5*time.Second)
	require.Contains(s.T(), s.readFile("/opt/datadog-agent/etc/datadog.yaml"), "api_key",
		"the stable configuration should still be whole")
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), wantLogLevel, config["log_level"], "the stable Agent should read the expected configuration")
}

// TestWatcherRevertWaitsForAnInstallerCommandMacOS pins the first half of the watcher's
// serialization (revertFromWatcher): a revert the watcher decides on while an installer command is
// running waits for that command's lock instead of acting on the configuration under it, then goes
// ahead once the command has finished and the experiment is still pending.
func (s *configMacOSSuite) TestWatcherRevertWaitsForAnInstallerCommandMacOS() {
	s.requireResting()
	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)

	deploymentID, _, release := s.startExperimentAndCrashIt("cfg-watcher-waits")
	defer release()

	// Without the lock the watcher reverts within seconds of the crash.
	s.requireExperimentStillDeployed(30 * time.Second)

	release()

	s.requireFailedOnHost(deploymentID)
	s.stopFailedExperimentRC()
	s.requireConfigurationIntact(before["log_level"])
}

// TestWatcherRevertRacingAPromoteKeepsAConfigurationMacOS reproduces the race the watcher's
// serialization closes: a watcher revert and a promote of the same experiment, both pending at
// once. Without the lock the two could interleave on the configuration directories -- the revert
// deleting through a handle on etc-exp while the promote swaps it into etc -- and leave no copy of
// either configuration. With it they run one after the other, and either order is a valid end:
//
//   - the promote first: it clears the deadline under the lock, the watcher re-reads it, finds its
//     experiment gone and does nothing, and the promoted configuration is the stable one;
//   - the watcher first: it puts the jobs back and keeps the experiment for Fleet's stop
//     (configExperiment.Fail), and the promote then promotes it, as on Linux, whose revert also
//     leaves the experiment configuration in place.
//
// Both are left waiting on the held lock and released together, so either may win. The promote is
// the installer command the daemon would run for a promote_experiment_config task, started on the
// host directly: the daemon checks the package state before executing a task, and that check waits
// on the held lock too, so a task would never get as far as the promote.
func (s *configMacOSSuite) TestWatcherRevertRacingAPromoteKeepsAConfigurationMacOS() {
	s.requireResting()
	before, err := s.Agent.Configuration()
	require.NoError(s.T(), err)

	deploymentID, expected, release := s.startExperimentAndCrashIt("cfg-watcher-promote-race")
	defer release()
	stableConfigBefore := expected.StableConfig

	const promoteLog = "/tmp/dd-e2e-promote.log"
	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf(
		`sudo sh -c '/opt/datadog-agent/embedded/bin/installer promote-config-experiment datadog-agent </dev/null >%s 2>&1 &'`,
		promoteLog))
	require.NoError(s.T(), err, "starting the promote should succeed")
	require.Eventually(s.T(), func() bool {
		out, err := s.Env().RemoteHost.Execute("pgrep -f 'installer [p]romote-config-experiment' || true")
		return err == nil && strings.TrimSpace(out) != ""
	}, 30*time.Second, time.Second, "the promote should be running, waiting for the installer lock")
	s.requireExperimentStillDeployed(10 * time.Second)

	release()

	require.Eventually(s.T(), func() bool {
		out, err := s.Env().RemoteHost.Execute("pgrep -f 'installer [p]romote-config-experiment' || true")
		return err == nil && strings.TrimSpace(out) == ""
	}, 3*time.Minute, 2*time.Second, "the promote should finish once the lock is released")
	if out, err := s.Env().RemoteHost.Execute("cat " + promoteLog + " || true"); err == nil {
		s.T().Logf("promote output:\n%s", out)
	}

	var stableConfig string
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.tryReadStatus()
		if !assert.NoError(c, err) || !assert.Len(c, status.Packages, 1) {
			return
		}
		pkg := status.Packages[0]
		assert.Empty(c, pkg.ExperimentConfigVersion, "nothing should be deployed once both have run")
		stableConfig = pkg.StableConfigVersion
	}, 3*time.Minute, 5*time.Second)

	switch stableConfig {
	case deploymentID:
		s.T().Log("the promote ran, before or after the watcher's revert; either way the configuration is whole")
		s.requireConfigurationIntact("debug")
	case stableConfigBefore:
		// Only possible if the promote failed after the watcher reverted.
		s.T().Log("the watcher won the installer lock and reverted; the promote did not promote")
		s.requireConfigurationIntact(before["log_level"])
	default:
		require.Failf(s.T(), "unexpected stable configuration", "got %q, want %q (promoted) or %q (reverted)",
			stableConfig, deploymentID, stableConfigBefore)
	}
}
