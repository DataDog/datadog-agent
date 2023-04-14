// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	innerSuite := NewSuite("e2eSuite", e2eSuite.createStack("default"))
	e2eSuite.Suite = innerSuite
	e2eSuite.updateEnvStack = e2eSuite.createStack("updateEnvStack")
	suite.Run(t, e2eSuite)
}

func (s *e2eSuite) Test1_DefaultEnv() {
	s.Env() // create the env if it doesn't exist
	s.Require().Equal("default", s.stackName)
	s.Require().Equal(1, s.runFctCallCount)
}

func (s *e2eSuite) Test2_UpdateEnv() {
	s.UpdateEnv(s.updateEnvStack)
	s.Env() // create the env if it doesn't exist
	s.Require().Equal("updateEnvStack", s.stackName)
	s.Require().Equal(2, s.runFctCallCount)
}

func (s *e2eSuite) Test3_UpdateEnv() {
	// As the env is the same as before this function does nothing
	// and runFctCallCount is not increment
	s.UpdateEnv(s.updateEnvStack)
	s.Env() // create the env if it doesn't exist
	s.Require().Equal("updateEnvStack", s.stackName)
	s.Require().Equal(2, s.runFctCallCount)

	s.UpdateEnv(s.updateEnvStack)
	s.Require().Equal(2, s.runFctCallCount)
}

func (suite *e2eSuite) createStack(stackName string) *StackDefinition[struct{}] {
	return &StackDefinition[struct{}]{
		EnvFactory: func(ctx *pulumi.Context) (*struct{}, error) {
			suite.stackName = stackName
			suite.runFctCallCount += 1
			return &struct{}{}, nil
		},
	}
}
