// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/params"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type e2eSuite struct {
	*Suite[struct{}]
	stackName       string
	runFctCallCount int
	updateEnvStack  *StackDefinition[struct{}]
}

func TestE2ESuite(t *testing.T) {
	e2eSuite := &e2eSuite{}
	innerSuite := newSuite("e2eSuite", e2eSuite.createStack("default"))
	e2eSuite.Suite = innerSuite
	e2eSuite.updateEnvStack = e2eSuite.createStack("updateEnvStack")
	suite.Run(t, e2eSuite)
}

func (suite *e2eSuite) Test1_DefaultEnv() {
	suite.Env() // create the env if it doesn't exist
	suite.Require().Equal("default", s.stackName)
	suite.Require().Equal(1, s.runFctCallCount)
}

func (suite *e2eSuite) Test2_UpdateEnv() {
	suite.UpdateEnv(s.updateEnvStack)
	suite.Env() // create the env if it doesn't exist
	suite.Require().Equal("updateEnvStack", suite.stackName)
	suite.Require().Equal(2, suite.runFctCallCount)
}

func (suite *e2eSuite) Test3_UpdateEnv() {
	// As the env is the same as before this function does nothing
	// and runFctCallCount is not increment
	suite.UpdateEnv(s.updateEnvStack)
	suite.Env() // create the env if it doesn't exist
	suite.Require().Equal("updateEnvStack", suite.stackName)
	suite.Require().Equal(2, suite.runFctCallCount)

	suite.UpdateEnv(suite.updateEnvStack)
	suite.Require().Equal(2, suite.runFctCallCount)
}

func (suite *e2eSuite) createStack(stackName string) *StackDefinition[struct{}] {
	return EnvFactoryStackDef(func(ctx *pulumi.Context) (*struct{}, error) {
		suite.stackName = stackName
		suite.runFctCallCount++
		return &struct{}{}, nil
	})
}

type skipDeleteOnFailureSuite struct {
	*Suite[struct{}]
	testsRun []string
}

// This function is used to check skipDeleteOnFailure works as expected
// which means a test must fail. This is the reason why the functino is not
// prefixed by `Test` and so not run.
func E2ESuiteSkipDeleteOnFailure(t *testing.T) {
	e2e2Suite := &skipDeleteOnFailureSuite{
		Suite: newSuite[struct{}]("SkipDeleteOnFailure", nil, params.WithSkipDeleteOnFailure()),
	}
	suite.Run(t, e2e2Suite)
	require.Equal(t, []string{"Test1"}, e2e2Suite.testsRun)
}

func (suite *skipDeleteOnFailureSuite) Test1() {
	suite.UpdateEnv(s.updateStack("Test1"))
}

func (suite *skipDeleteOnFailureSuite) Test2() {
	suite.Assert().Fail("Simulate a failure")
	suite.UpdateEnv(suite.updateStack("Test2"))
}

func (suite *skipDeleteOnFailureSuite) Test3() {
	suite.UpdateEnv(s.updateStack("Test3"))
}

func (suite *skipDeleteOnFailureSuite) updateStack(testName string) *StackDefinition[struct{}] {
	return EnvFactoryStackDef(func(ctx *pulumi.Context) (*struct{}, error) {
		suite.testsRun = append(suite.testsRun, testName)
		return &struct{}{}, nil
	})
}

func newSuite[Env any](stackName string, stackDef *StackDefinition[Env], options ...params.Option) *Suite[Env] {
	testSuite := Suite[Env]{}
	testSuite.initSuite(stackName, stackDef, options...)
	return &testSuite
}
