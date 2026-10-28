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
	"testing"

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
	// We call reconcileEnv directly here instead of UpdateEnv: UpdateEnv records an
	// assertion failure before panicking on reconcile errors (added in PR #35167 for
	// the skipDeleteOnFailure flow), which would mark this test as failed even though
	// the failure is the expected outcome we're asserting against.
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

// TestTearDownSuiteIdempotent verifies the cleanupCalled guard:
//   - testify's post-test defer runs TearDownSuite once, calling Destroy.
//   - A second direct TearDownSuite call observes cleanupCalled and short-circuits
//     without calling Destroy again.
func TestTearDownSuiteIdempotent(t *testing.T) {
	p := &testProvisioner{}
	p.On("ID").Return("test")
	p.On("Provision", mock.Anything, mock.Anything, mock.Anything).Return(makeTestEnvResources(), nil)
	p.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := &testNoOpSuite{}
	Run(t, s, WithProvisioner(p))

	p.AssertNumberOfCalls(t, "Destroy", 1)
	require.True(t, s.cleanupCalled, "cleanupCalled should be true after TearDownSuite ran")

	// Second TearDownSuite call must observe the guard and not call Destroy again.
	s.TearDownSuite()
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
