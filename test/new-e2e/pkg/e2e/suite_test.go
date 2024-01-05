// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/DataDog/test-infra-definitions/components"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testTypeOutput struct {
	components.JSONImporter

	MyField string `json:"myField"`
}

type testTypeWrapper struct {
	testTypeOutput

	unrelatedField string //nolint:unused, mimic actual struct to validate reflection code
}

var _ Initializable = &testTypeWrapper{}

func (t *testTypeWrapper) Init(Context) error {
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

func testRawResources(key, value string) RawResources {
	return RawResources{key: []byte(fmt.Sprintf(`{"myField":"%s"}`, value))}
}

func TestCreateEnv(t *testing.T) {
	suite := &testSuite{}

	env, envFields, envValues, err := suite.createEnv()
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

var _ UntypedProvisioner = &testProvisioner{}

func (m *testProvisioner) ID() string {
	args := m.Called()
	return args.Get(0).(string)
}

func (m *testProvisioner) Provision(arg0 context.Context, arg1 string, arg2 io.Writer) (RawResources, error) {
	args := m.Called(arg0, arg1, arg2)
	return args.Get(0).(RawResources), args.Error(1)
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

	Run(t, &testSuiteWithTests{permanentProvisioner: permanentProvisioner, tempProvisioner: tempProvisioner}, WithProvisioner(permanentProvisioner), WithProvisioner(tempProvisioner))
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
	// `Provide` will be called again on permanent provisioner
	// The call will panic because missing resource `myWrapper2`
	s.tempProvisioner.On("Destroy", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	s.Assert().PanicsWithError(
		"unable to build env: *e2e.testEnv from resources for stack: e2e-testSuiteWithTests-e3e6e49fc651fcb8, err: expected resource named: Wrapper2 with key: myWrapper2 but not returned by provisioners",
		func() { s.UpdateEnv(s.permanentProvisioner) },
	)

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
}
