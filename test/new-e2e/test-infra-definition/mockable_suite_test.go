// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type mySuite struct {
	MockableSuite[e2e.AgentEnv] // Type is not the same
}

func TestMySuiteSuite(t *testing.T) {
	e2e.Run(t,
		&mySuite{MockableSuite: myCustomLocalEnv()}, // The suite creation is a little different
		e2e.AgentStackDef(), params.WithDevMode())
}

// Tests are exactly the same
func (v *mySuite) TestWithImageName() {
	output := v.Env().VM.Execute("hostname -I")
	version := v.Env().Agent.Version()
	assert.Equal(v.T(), "", output)
	assert.Equal(v.T(), "", version)

}

// Define how the local AgentEnv is created. Can be reused accros tests
func myCustomLocalEnv() MockableSuite[e2e.AgentEnv] {
	isLocal := true
	return NewMockableSuite(isLocal, func(t *testing.T) (*e2e.AgentEnv, error) {
		vm, err := client.NewVMClient(t, &utils.Connection{Host: "10.1.61.181", User: "ubuntu"}, os.UnixType)
		if err != nil {
			return nil, err
		}
		// 10.1.61.181 contains an existing agent
		agent, err := client.NewAgentClient(t, vm, os.NewUbuntu(), false)
		if err != nil {
			return nil, err
		}
		return &e2e.AgentEnv{
			VM:    vm,
			Agent: agent,
		}, nil
	}, func() {
		fmt.Println("Reset")
		// Handle reset between test
	})
}

// -------------------------------
// The code below is shared and can be used for another test suite.
// -------------------------------
type MockableSuite[Env any] interface {
	e2e.SuiteConstraint[Env]
	Env() *Env
	BeforeTest(suiteName, testName string)
	AfterTest(suiteName, testName string)
	SetupSuite()
	TearDownSuite()
}

type mockableSuite[Env any] struct {
	suite.Suite
	localEnvFactory func(t *testing.T) (*Env, error)
	beforeTest      func()
	env             *Env
}

func NewMockableSuite[Env any](isLocal bool, localEnvFactory func(t *testing.T) (*Env, error), beforeTest func()) MockableSuite[Env] {
	if isLocal {
		return &mockableSuite[Env]{beforeTest: beforeTest, localEnvFactory: localEnvFactory}
	}
	return &e2e.Suite[Env]{}
}

func (mockableSuite *mockableSuite[Env]) Env() *Env {
	if mockableSuite.env == nil {
		var err error
		mockableSuite.env, err = mockableSuite.localEnvFactory(mockableSuite.T())
		mockableSuite.Require().NoError(err)
	}
	return mockableSuite.env
}

func (mockableSuite *mockableSuite[Env]) BeforeTest(suiteName, testName string) {
	mockableSuite.beforeTest()
}

func (mockableSuite *mockableSuite[Env]) InitSuite(stackName string, stackDef *e2e.StackDefinition[Env], options ...params.Option) {
	// Nothing
}

func (mockableSuite *mockableSuite[Env]) AfterTest(suiteName, testName string) {
	// Nothing
}
func (mockableSuite *mockableSuite[Env]) SetupSuite() {
	// Nothing
}

func (mockableSuite *mockableSuite[Env]) TearDownSuite() {
	// Nothing
}
