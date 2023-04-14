// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides tools to manage environments and run E2E tests.
// See [Suite] for an example of the usage.
package e2e

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/suite"
)

const (
	deleteTimeout = 30 * time.Minute
)

// Suite manages the environment creation and runs E2E tests.
// It is implemented as a [testify Suite].
// Example of usage:
//
//	  type MyEnv struct {
//		   VM *client.VM
//	  }
//	  type vmSuite struct {
//		   *Suite[MyEnv]
//	  }
//
//	  func TestE2ESuite(t *testing.T) {
//		   suite.Run(t, &vmSuite{Suite: NewSuite("my-test", &StackDefinition[MyEnv]{
//			 EnvFactory: func(ctx *pulumi.Context) (*MyEnv, error) {
//				vm, err := ec2vm.NewUnixLikeEc2VM(ctx, ec2vm.WithOS(os.AmazonLinuxOS, commonos.AMD64Arch))
//				if err != nil {
//					return nil, err
//				}
//				return &MyEnv{
//					VM: client.NewVM(vm),
//				}, nil
//			  },
//		   })})
//	  }
//
// Suite leverages pulumi features to compute the differences between the previous
// environment and the new one to make environment updates faster.
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
type Suite[Env any] struct {
	suite.Suite

	stackName       string
	defaultStackDef *StackDefinition[Env]
	currentStackDef *StackDefinition[Env]

	// These fields are initialized in SetupSuite
	env  *Env
	auth client.Authentification

	isUpdateEnvCalledInThisTest bool

	// Setting DevMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	DevMode bool
}

type StackDefinition[Env any] struct {
	EnvFactory func(ctx *pulumi.Context) (*Env, error)
	ConfigMap  runner.ConfigMap
}

// NewSuite creates a new Suite.
// stackName is the name of the stack and should be unique across suites.
// stackDef is the stack definition.
// options are optional parameters for example [e2e.KeepEnv].
func NewSuite[Env any](stackName string, stackDef *StackDefinition[Env], options ...func(*Suite[Env])) *Suite[Env] {
	testSuite := Suite[Env]{
		stackName:       stackName,
		defaultStackDef: stackDef,
	}

	for _, o := range options {
		o(&testSuite)
	}

	return &testSuite
}

// Env returns the current environment.
// In order to improve the efficiency, this function behaves as follow:
//   - It creates the default environment if no environment exists. It happens only during the first call of the test suite.
//   - It restores the default environment if UpdateEnv was not already be called during this test.
//     This avoid having to restore the default environment for each test even if UpdateEnv immedialy
//     overrides this environment.
func (suite *Suite[Env]) Env() *Env {
	if suite.env == nil || !suite.isUpdateEnvCalledInThisTest {
		suite.UpdateEnv(suite.defaultStackDef)
	}
	return suite.env
}

func (suite *Suite[Env]) BeforeTest(suiteName, testName string) {
	suite.isUpdateEnvCalledInThisTest = false
}

// SetupSuite method will run before the tests in the suite are run.
// This function is called by [testify Suite].
// Note: Having initialization code in this function allows `NewSuite` to not
// return an error in order to write a single line for
// `suite.Run(t, &vmSuite{Suite: e2e.NewSuite(...)})`
func (suite *Suite[Env]) SetupSuite() {
	// Check if the Env type is correct otherwise raises an error before creating the env.
	err := client.CheckEnvStructValid[Env]()
	suite.Require().NoError(err)
}

// HandleStats method is run after all the tests in the suite have been run.
// and after TearDownSuite has been run.
// This function is called by [testify Suite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite[Env]) HandleStats(string, stats *suite.SuiteInformation) {
	if runner.GetProfile().AllowDevMode() && suite.DevMode {
		return
	}

	skipDelete, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.SkipDeleteOnFailure, false)
	if !stats.Passed() && skipDelete {
		return
	}

	// TODO: Implement retry on delete
	ctx, cancel := context.WithTimeout(context.Background(), deleteTimeout)
	defer cancel()
	err := infra.GetStackManager().DeleteStack(ctx, suite.stackName)
	if err != nil {
		suite.T().Errorf("unable to delete stack: %s, err :%v", suite.stackName, err)
		suite.T().Fail()
	}
}

func createEnv[Env any](suite *Suite[Env], stackDef *StackDefinition[Env]) (*Env, auto.UpResult, error) {
	var env *Env
	ctx := context.Background()

	_, stackOutput, err := infra.GetStackManager().GetStack(
		ctx,
		suite.stackName,
		stackDef.ConfigMap,
		func(ctx *pulumi.Context) error {
			var err error
			env, err = stackDef.EnvFactory(ctx)
			return err
		}, false)

	return env, stackOutput, err
}

func (suite *Suite[Env]) UpdateEnv(stackDef *StackDefinition[Env]) {
	if stackDef != suite.currentStackDef {
		env, upResult, err := createEnv(suite, stackDef)
		suite.Require().NoError(err)
		err = client.CallStackInitializers(&suite.auth, env, upResult)
		suite.Require().NoError(err)
		suite.env = env
		suite.currentStackDef = stackDef
	}
	suite.isUpdateEnvCalledInThisTest = true
}
