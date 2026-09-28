// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2eunit

package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

type testTypeOutput struct {
	components.JSONImporter

	MyField string `json:"myField"`
}

type testTypeWrapper struct {
	testTypeOutput

	unrelatedField string //nolint:unused // mimic actual struct to validate reflection code
}

var _ common.Initializable = &testTypeWrapper{}

func (t *testTypeWrapper) Init(common.Context) error {
	return nil
}

func (t *testTypeWrapper) GetMyField() string {
	return t.MyField
}

type testEnv struct {
	Wrapper1 *testTypeWrapper `import:"myWrapper1"`
	Wrapper2 *testTypeWrapper `import:"myWrapper2"`
}

type testSuite struct {
	BaseSuite[testEnv]
}

func testRawResources(key, value string) provisioners.RawResources {
	return provisioners.RawResources{key: []byte(fmt.Sprintf(`{"myField":"%s"}`, value))}
}

func TestCreateEnv(t *testing.T) {
	suite := &testSuite{}

	env, envFields, envValues, err := environments.CreateEnv[testEnv]()
	require.NoError(t, err)

	testResources := testRawResources("myWrapper1", "myValue")
	testResources.Merge(testRawResources("myWrapper2", "myValue"))
	err = suite.buildEnvFromResources(testResources, envFields, envValues)

	require.NoError(t, err)
	require.Equal(t, "myValue", env.Wrapper1.GetMyField())
}

type testProvisioner struct {
	mock.Mock
}

var _ provisioners.UntypedProvisioner = &testProvisioner{}

func (m *testProvisioner) ID() string {
	args := m.Called()
	return args.Get(0).(string)
}

func (m *testProvisioner) Provision(arg0 context.Context, arg1 string, arg2 io.Writer) (provisioners.RawResources, error) {
	args := m.Called(arg0, arg1, arg2)
	return args.Get(0).(provisioners.RawResources), args.Error(1)
}

func (m *testProvisioner) Destroy(arg0 context.Context, arg1 string, arg2 io.Writer) error {
	args := m.Called(arg0, arg1, arg2)
	return args.Error(0)
}

type testSuiteWithTests struct {
	BaseSuite[testEnv]

	permanentProvisioner *testProvisioner
	tempProvisioner      *testProvisioner
}

func TestProvisioningSequence(t *testing.T) {
	// Permanent provisioner is always going to be there
	permanentProvisioner := &testProvisioner{}
	permanentProvisioner.On("ID").Return("permanent")
	permanentProvisioner.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(testRawResources("myWrapper1", "permanent"), nil)

	// Temp provisioner is going to be removed in some tests
	tempProvisioner := &testProvisioner{}
	tempProvisioner.On("ID").Return("temp")
	tempProvisioner.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(testRawResources("myWrapper2", "temp"), nil)

	s := &testSuiteWithTests{permanentProvisioner: permanentProvisioner, tempProvisioner: tempProvisioner}
	Run(t, s, WithProvisioner(permanentProvisioner), WithProvisioner(tempProvisioner))

	// TearDownSuite ran after the last test method. Verify final Destroy counts:
	// permanent destroyed once (TearDownSuite); temp destroyed twice (once mid-suite
	// by UpdateEnv in TestOrderA, then re-provisioned at the start of TestOrderB and
	// destroyed again by TearDownSuite).
	permanentProvisioner.AssertNumberOfCalls(t, "Destroy", 1)
	tempProvisioner.AssertNumberOfCalls(t, "Destroy", 2)
}

func (s *testSuiteWithTests) TestOrderA() {
	s.permanentProvisioner.AssertExpectations(s.T())
	s.permanentProvisioner.AssertNumberOfCalls(s.T(), "Provision", 1)
	s.tempProvisioner.AssertExpectations(s.T())
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Provision", 1)

	// Nothing should happen, same objects
	s.UpdateEnv(s.permanentProvisioner, s.tempProvisioner)

	s.permanentProvisioner.AssertExpectations(s.T())
	s.permanentProvisioner.AssertNumberOfCalls(s.T(), "Provision", 1)
	s.tempProvisioner.AssertExpectations(s.T())
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Provision", 1)

	// Remove temp provisioner, destroy should be called.
	// `Provide` will be called again on permanent provisioner.
	// The call will fail because of missing resource `myWrapper2`.
	//
	// We call reconcileEnv directly here instead of UpdateEnv: UpdateEnv calls
	// bs.T().Fail() before panicking on reconcile errors (added in PR #35167 for the
	// skipDeleteOnFailure flow), which would mark this test as failed even though the
	// failure is the expected outcome we're asserting against.
	s.tempProvisioner.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	expectedErr := fmt.Sprintf(
		"unable to build env: *e2e.testEnv from resources for stack: %s, err: expected resource named: Wrapper2 with key: myWrapper2 but not returned by provisioners",
		s.params.stackName,
	)
	err := s.reconcileEnv(provisioners.ProvisionerMap{s.permanentProvisioner.ID(): s.permanentProvisioner})
	s.Assert().EqualError(err, expectedErr)

	s.permanentProvisioner.AssertExpectations(s.T())
	s.permanentProvisioner.AssertNumberOfCalls(s.T(), "Provision", 2)
	s.tempProvisioner.AssertExpectations(s.T())
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Provision", 1)
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Destroy", 1)

	// As UpdateEnv failed, the `currentProvisioners` have not been updated
	// so the next test should not call provisioners again.
	// As we want that to happen, we'll simulate that by patching manually the `currentProvisioners`.
	delete(s.currentProvisioners, s.tempProvisioner.ID())
}

func (s *testSuiteWithTests) TestOrderB() {
	// In this test, the original provisioners will be called again, restoring everything
	s.permanentProvisioner.AssertNumberOfCalls(s.T(), "Provision", 3)
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Provision", 2)
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Destroy", 1)
}

func (s *testSuiteWithTests) TestOrderC() {
	// The provisioners were not changed, so nothing should happen
	s.permanentProvisioner.AssertNumberOfCalls(s.T(), "Provision", 3)
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Provision", 2)
	s.tempProvisioner.AssertNumberOfCalls(s.T(), "Destroy", 1)

	// Register Destroy expectation on permanent here (last test method before
	// TearDownSuite). Doing it earlier would trip mid-test AssertExpectations calls,
	// since AssertExpectations demands every registered On() to have fired.
	s.permanentProvisioner.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)
}

// testNoOpSuite is a BaseSuite with a single no-op test method. The no-op is required
// because testify's suite.Run skips SetupSuite/TearDownSuite when no test methods exist.
type testNoOpSuite struct {
	BaseSuite[testEnv]
}

func (s *testNoOpSuite) TestNoOp() {}

func makeTestEnvResources() provisioners.RawResources {
	resources := testRawResources("myWrapper1", "x")
	resources.Merge(testRawResources("myWrapper2", "y"))
	return resources
}

// TestTearDownSuiteIdempotent verifies the teardown guard:
//   - testify's post-test defer runs TearDownSuite once, calling Destroy.
//   - A second direct TearDownSuite call blocks on the sync.Once and returns
//     without calling Destroy again.
func TestTearDownSuiteIdempotent(t *testing.T) {
	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(makeTestEnvResources(), nil)
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := &testNoOpSuite{}
	Run(t, s, WithProvisioner(p))

	p.AssertNumberOfCalls(t, "Destroy", 1)
	require.True(t, s.teardownStarted.Load(), "teardownStarted should be true after TearDownSuite ran")

	// Second TearDownSuite call must observe the guard and not call Destroy again.
	s.TearDownSuite()
	p.AssertNumberOfCalls(t, "Destroy", 1)
}

// TestTearDownSuiteConcurrentCallsRunOnce verifies the concurrency contract of
// the teardown guard: when two goroutines call TearDownSuite at the same time,
// the teardown body runs exactly once (Destroy once) and the second caller
// blocks until the first teardown finishes instead of returning early.
func TestTearDownSuiteConcurrentCallsRunOnce(t *testing.T) {
	destroyStarted := make(chan struct{})
	destroyRelease := make(chan struct{})
	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(makeTestEnvResources(), nil)
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil).
		Run(func(mock.Arguments) {
			close(destroyStarted)
			<-destroyRelease
		})

	s := &testNoOpSuite{}
	s.init([]SuiteOption{WithProvisioner(p)}, s)
	s.SetT(t)

	done := make(chan struct{}, 2)
	go func() { s.TearDownSuite(); done <- struct{}{} }()
	<-destroyStarted // first teardown is now inside Destroy

	go func() { s.TearDownSuite(); done <- struct{}{} }()
	select {
	case <-done:
		t.Fatal("second TearDownSuite returned while the first teardown was still running")
	case <-time.After(200 * time.Millisecond):
	}

	close(destroyRelease)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("TearDownSuite did not return")
		}
	}
	p.AssertNumberOfCalls(t, "Destroy", 1)
}

// TestTCleanupHookIsNoOpAfterNormalTeardown verifies the happy-path behavior of the
// t.Cleanup hook registered by SetupSuite:
//   - Suite runs as a sub-test so the hook fires when the sub-test completes.
//   - testify already ran TearDownSuite normally, so cleanupCalled is true.
//   - The hook observes the guard and must not call Destroy a second time.
func TestTCleanupHookIsNoOpAfterNormalTeardown(t *testing.T) {
	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(makeTestEnvResources(), nil)
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	t.Run("inner", func(subT *testing.T) {
		Run(subT, &testNoOpSuite{}, WithProvisioner(p))
	})

	// The t.Cleanup hook fired after the sub-test completed. If it had erroneously
	// re-run cleanup, Destroy would have been called twice.
	p.AssertNumberOfCalls(t, "Destroy", 1)
}

func TestParseTeardownBudget(t *testing.T) {
	for value, expected := range map[string]struct {
		budget time.Duration
		err    bool
	}{
		"":     {defaultTeardownBudget, false},
		"5m":   {5 * time.Minute, false},
		"nope": {defaultTeardownBudget, true},
		"0":    {defaultTeardownBudget, true},
		"0s":   {defaultTeardownBudget, true},
		"-5m":  {defaultTeardownBudget, true},
	} {
		budget, err := parseTeardownBudget(value)
		require.Equal(t, expected.budget, budget, "value %q", value)
		if expected.err {
			require.Error(t, err, "value %q", value)
		} else {
			require.NoError(t, err, "value %q", value)
		}
	}
}

type testDeadlineOverrideSuite struct {
	BaseSuite[testEnv]

	overrideRan chan struct{}
	baseDone    chan struct{}
}

func (s *testDeadlineOverrideSuite) TearDownSuite() {
	close(s.overrideRan)
	s.BaseSuite.TearDownSuite()
	close(s.baseDone)
}

type testDeadlineAbandonSuite struct {
	BaseSuite[testEnv]

	started     chan struct{}
	reachedBase chan struct{}
	abandon     func()
}

func (s *testDeadlineAbandonSuite) TearDownSuite() {
	close(s.started)
	s.abandon()
	s.BaseSuite.TearDownSuite()
	close(s.reachedBase)
}

// TestDeadlineTeardownRunsDestroyDespiteSkipDeleteOnFailure verifies deadline
// semantics: on the deadline path the stack is deleted even when
// skipDeleteOnFailure would otherwise keep it after a failure.
func TestDeadlineTeardownRunsDestroyDespiteSkipDeleteOnFailure(t *testing.T) {
	t.Setenv("REMOTE_STACK_CLEANING", "") // force the local destroy path

	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := &testNoOpSuite{}
	s.init([]SuiteOption{WithProvisioner(p)}, s)
	s.SetT(t)
	s.params.skipDeleteOnFailure = true
	firstFail := "TestSuite.TestSomething"
	s.firstFailTest.Store(&firstFail)

	s.runDeadlineTeardown(t)

	p.AssertNumberOfCalls(t, "Destroy", 1)
}

func TestNormalTeardownAfterDeadlineDoesNotKeepFailedStack(t *testing.T) {
	t.Setenv("REMOTE_STACK_CLEANING", "")
	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := &testNoOpSuite{}
	s.init([]SuiteOption{WithProvisioner(p)}, s)
	s.SetT(t)
	s.suiteT = t
	s.e2eDeadline = time.Now().Add(-time.Second)
	s.params.skipDeleteOnFailure = true
	firstFail := "Initial provisioning SetupSuite"
	s.firstFailTest.Store(&firstFail)

	// Provisioning cancellation can reach ordinary cleanup before the watchdog runs.
	s.TearDownSuite()
	s.runDeadlineTeardown(t)

	p.AssertNumberOfCalls(t, "Destroy", 1)
}

// TestDeadlineTeardownDispatchesThroughOverride verifies the deadline teardown
// dispatches through the derived suite's TearDownSuite override, so overrides
// that clean resources the stackcleaner can't see also run.
func TestDeadlineTeardownDispatchesThroughOverride(t *testing.T) {
	t.Setenv("REMOTE_STACK_CLEANING", "") // force the local destroy path

	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := &testDeadlineOverrideSuite{overrideRan: make(chan struct{}), baseDone: make(chan struct{})}
	s.init([]SuiteOption{WithProvisioner(p)}, s)
	s.SetT(t)

	s.runDeadlineTeardown(t)

	select {
	case <-s.overrideRan:
	case <-time.After(30 * time.Second):
		t.Fatal("deadline teardown did not dispatch through the TearDownSuite override")
	}
	select {
	case <-s.baseDone:
	case <-time.After(30 * time.Second):
		t.Fatal("base TearDownSuite did not complete")
	}
	p.AssertNumberOfCalls(t, "Destroy", 1)
}

// TestDeadlineTeardownFallsBackWhenOverrideNeverReachesBase verifies the
// safety net: when a TearDownSuite override abandons the teardown goroutine
// (require/FailNow Goexits it, or it panics), the deferred fallback still runs
// the base teardown.
func TestDeadlineTeardownFallsBackWhenOverrideNeverReachesBase(t *testing.T) {
	for name, abandon := range map[string]func(){
		// require/FailNow in an override Goexits the teardown goroutine; simulate
		// the Goexit directly so the test's own T is not failed.
		"override goexits": runtime.Goexit,
		"override panics":  func() { panic("override failed") },
	} {
		t.Run(name, func(subT *testing.T) {
			subT.Setenv("REMOTE_STACK_CLEANING", "") // force the local destroy path

			p := &testProvisioner{}
			p.On("ID").Return("test")
			p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			s := &testDeadlineAbandonSuite{started: make(chan struct{}), reachedBase: make(chan struct{}), abandon: abandon}
			s.init([]SuiteOption{WithProvisioner(p)}, s)
			s.SetT(subT)

			done := make(chan struct{})
			go func() {
				defer close(done)
				s.runDeadlineTeardown(subT)
			}()
			<-s.started

			select {
			case <-done:
			case <-time.After(30 * time.Second):
				subT.Fatal("runDeadlineTeardown did not return")
			}

			select {
			case <-s.reachedBase:
				subT.Fatal("override reached the base teardown despite abandoning")
			default:
			}
			p.AssertNumberOfCalls(subT, "Destroy", 1)
		})
	}
}

// TestDeadlineTeardownWaitsForInFlightProvisioning verifies the deadline
// teardown joins an in-flight reconcileEnv (via the provisioning mutex) instead
// of racing it: it must not start tearing down while provisioning holds the
// mutex, and must proceed once provisioning completes.
func TestDeadlineTeardownWaitsForInFlightProvisioning(t *testing.T) {
	t.Setenv("REMOTE_STACK_CLEANING", "") // force the local destroy path

	p := &testProvisioner{}
	p.On("ID").Return("test")
	inProvision := make(chan struct{})
	releaseProvision := make(chan struct{})
	p.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(makeTestEnvResources(), nil).
		Run(func(mock.Arguments) { close(inProvision); <-releaseProvision })
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := &testNoOpSuite{}
	s.init([]SuiteOption{WithProvisioner(p)}, s)
	s.SetT(t)

	reconcileDone := make(chan struct{})
	go func() {
		defer close(reconcileDone)
		if err := s.reconcileEnv(provisioners.ProvisionerMap{p.ID(): p}); err != nil {
			t.Errorf("reconcileEnv: %v", err)
		}
	}()
	<-inProvision // reconcileEnv now holds the provisioning mutex, inside Provision

	teardownDone := make(chan struct{})
	go func() {
		defer close(teardownDone)
		s.runDeadlineTeardown(t)
	}()

	select {
	case <-teardownDone:
		t.Fatal("deadline teardown finished while provisioning was still in flight")
	case <-time.After(300 * time.Millisecond):
	}

	close(releaseProvision)
	<-reconcileDone
	select {
	case <-teardownDone:
	case <-time.After(30 * time.Second):
		t.Fatal("deadline teardown did not finish after provisioning completed")
	}
	p.AssertNumberOfCalls(t, "Destroy", 1)
}

// TestArmDeadlineWatchdog verifies the arming conditions: no watchdog locally,
// and a timer set to the go test deadline minus the teardown budget when
// armed. (The no-deadline case is covered by the "no-timeout" subprocess
// scenario of TestDeadlineWatchdogProcess.)
func TestArmDeadlineWatchdog(t *testing.T) {
	t.Run("not armed locally", func(subT *testing.T) {
		subT.Setenv("GITLAB_CI", "")
		subT.Setenv("REMOTE_STACK_CLEANING", "")
		s := &testNoOpSuite{}
		s.init(nil, s)
		s.armDeadlineWatchdog(subT, time.Minute)
		require.Nil(subT, s.deadlineTimer)
		require.True(subT, s.e2eDeadline.IsZero())
	})

	t.Run("armed", func(subT *testing.T) {
		subT.Setenv("GITLAB_CI", "true")
		subT.Setenv("REMOTE_STACK_CLEANING", "true")
		s := &testNoOpSuite{}
		s.init(nil, s)
		deadline, ok := subT.Deadline()
		require.True(subT, ok, "expected a go test deadline")
		budget := time.Second
		s.armDeadlineWatchdog(subT, budget)
		require.NotNil(subT, s.deadlineTimer)
		require.True(subT, s.deadlineTimer.Stop(), "watchdog timer should be armed and still running")
		require.Equal(subT, deadline.Add(-budget), s.e2eDeadline)
		require.Same(subT, subT, s.suiteT)
	})
}

// TestDeadlineTeardownCapsTeardownOperations verifies teardown operations run
// under a context capped at the remaining teardown budget (budget minus the
// minute reserved for the stackcleaner request), so a slow Diagnose or destroy
// cannot eat the time the stackcleaner request needs.
func TestDeadlineTeardownCapsTeardownOperations(t *testing.T) {
	t.Setenv("REMOTE_STACK_CLEANING", "") // force the local destroy path
	t.Setenv("E2E_TEARDOWN_BUDGET", "2m")

	p := &testProvisioner{}
	p.On("ID").Return("test")
	var destroyCtx context.Context
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil).
		Run(func(args mock.Arguments) { destroyCtx = args.Get(0).(context.Context) })

	s := &testNoOpSuite{}
	s.init([]SuiteOption{WithProvisioner(p)}, s)
	s.SetT(t)

	s.runDeadlineTeardown(t)

	deadline, ok := destroyCtx.Deadline()
	require.True(t, ok, "Destroy should run with a deadline-capped context")
	require.WithinDuration(t, time.Now().Add(time.Minute), deadline, 10*time.Second)
}

// testDeadlineChildSuite is run by TestDeadlineWatchdogProcess inside a child
// process. Its provisioner and teardown write marker files asserted by the
// parent test.
type testDeadlineChildSuite struct {
	BaseSuite[testEnv]

	dir string
}

func (s *testDeadlineChildSuite) marker(name string) {
	_ = os.WriteFile(filepath.Join(s.dir, name), []byte("done"), 0o644)
}

func (s *testDeadlineChildSuite) TearDownSuite() {
	s.marker("override")
	s.BaseSuite.TearDownSuite()
	s.marker("teardown-done")
}

func (s *testDeadlineChildSuite) TestNoOp() {}

type testDeadlineSleepSuite struct {
	testDeadlineChildSuite
}

func (s *testDeadlineSleepSuite) TestSleep() {
	time.Sleep(30 * time.Second)
}

// TestDeadlineWatchdogProcess exercises the deadline watchdog end to end in
// subprocesses (a watchdog goroutine failing a live T poisons the in-process
// test): the guard refusing to provision when the budget does not fit before
// the go test deadline, the watchdog tearing a running suite down at the e2e
// deadline, and the teardown-only run being exempt from both.
func TestDeadlineWatchdogProcess(t *testing.T) {
	if os.Getenv("E2E_DEADLINE_TEST_SCENARIO") != "" {
		deadlineWatchdogScenario(t)
		return
	}

	scenarios := []struct {
		name        string
		budget      string
		goTestFlags []string
		extraEnv    []string
	}{
		// 1000h never fits before any deadline, so the guard always fires.
		{name: "guard", budget: "1000h", goTestFlags: []string{"-test.timeout=10m"}},
		{name: "watchdog", budget: "5s", goTestFlags: []string{"-test.timeout=10s"}},
		{name: "teardown-only", budget: "1000h", goTestFlags: []string{"-test.timeout=10m"}, extraEnv: []string{"E2E_TEARDOWN_ONLY=true"}},
		// -test.timeout=0 means no go test deadline: the watchdog must not arm
		// (even though the 1000h budget would never fit), so the suite runs and
		// passes normally.
		{name: "no-timeout", budget: "1000h", goTestFlags: []string{"-test.timeout=0"}},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(subT *testing.T) {
			dir := subT.TempDir()
			args := append([]string{
				"-test.run=^TestDeadlineWatchdogProcess$",
				"-test.v=true",
			}, sc.goTestFlags...)
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(),
				"E2E_DEADLINE_TEST_SCENARIO="+sc.name,
				"E2E_DEADLINE_TEST_DIR="+dir,
				"E2E_TEARDOWN_BUDGET="+sc.budget,
				"E2E_API_KEY=dummy",
				"E2E_APP_KEY=dummy",
				"GITLAB_CI=true",
				"REMOTE_STACK_CLEANING=true",
			)
			cmd.Env = append(cmd.Env, sc.extraEnv...)
			out, err := cmd.CombinedOutput()
			marker := func(name string) string { return filepath.Join(dir, name) }

			switch sc.name {
			case "guard":
				require.Error(subT, err, "child should fail: %s", out)
				require.Contains(subT, string(out), "not enough time left before the go test deadline")
				require.NoFileExists(subT, marker("provisioned"), "the guard must fail before provisioning")
			case "watchdog":
				require.Error(subT, err, "child should fail: %s", out)
				require.Contains(subT, string(out), "e2e deadline reached while running")
				require.FileExists(subT, marker("provisioned"), "suite should have provisioned before the deadline")
				require.FileExists(subT, marker("override"), "deadline teardown should dispatch through the override")
				require.FileExists(subT, marker("teardown-done"), "base teardown should have completed")
			case "teardown-only":
				require.NoError(subT, err, "child should pass: %s", out)
				require.NotContains(subT, string(out), "not enough time left")
				require.NoFileExists(subT, marker("provisioned"), "teardown-only must not provision")
			case "no-timeout":
				require.NoError(subT, err, "child should pass: %s", out)
				require.NotContains(subT, string(out), "not enough time left")
				require.FileExists(subT, marker("provisioned"), "suite should run normally without a deadline")
			}
		})
	}
}

func deadlineWatchdogScenario(t *testing.T) {
	dir := os.Getenv("E2E_DEADLINE_TEST_DIR")
	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(makeTestEnvResources(), nil).
		Run(func(mock.Arguments) { _ = os.WriteFile(filepath.Join(dir, "provisioned"), []byte("done"), 0o644) })
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	if os.Getenv("E2E_DEADLINE_TEST_SCENARIO") == "watchdog" {
		Run(t, &testDeadlineSleepSuite{testDeadlineChildSuite{dir: dir}}, WithProvisioner(p))
		return
	}
	Run(t, &testDeadlineChildSuite{dir: dir}, WithProvisioner(p))
}
