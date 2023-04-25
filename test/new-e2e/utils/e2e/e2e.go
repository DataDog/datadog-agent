// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides tools to manage environments and run E2E tests.
// See [Suite] for an example of the usage.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
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

	stackName string
	stackDef  *StackDefinition[Env]

	// These fields are initialized in SetupSuite
	Env  *Env
	auth client.Authentification

	// Setting DevMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	DevMode bool
}

type StackDefinition[Env any] struct {
	EnvFactory func(ctx *pulumi.Context) (*Env, error)
	ConfigMap  runner.ConfigMap
}

type suiteConstraint[Env any] interface {
	suite.TestingSuite
	initSuite(stackName string, stackDef *StackDefinition[Env], options ...func(*Suite[Env]))
}

func Run[Env any, T suiteConstraint[Env]](t *testing.T, e2eSuite T, stackName string, stackDef *StackDefinition[Env], options ...func(*Suite[Env])) {
	e2eSuite.initSuite(stackName, stackDef, options...)
	suite.Run(t, e2eSuite)
}

// NewSuite creates a new Suite.
// stackName is the name of the stack and should be unique across suites.
// stackDef is the stack definition.
// options are optional parameters for example [e2e.KeepEnv].
func NewSuite[Env any](stackName string, stackDef *StackDefinition[Env], options ...func(*Suite[Env])) *Suite[Env] {
	testSuite := Suite[Env]{}
	testSuite.initSuite(stackName, stackDef, options...)
	return &testSuite
}

func (suite *Suite[Env]) initSuite(stackName string, stackDef *StackDefinition[Env], options ...func(*Suite[Env])) {
	suite.stackName = stackName
	suite.stackDef = stackDef

	for _, o := range options {
		o(suite)
	}
}

// SetupSuite method will run before the tests in the suite are run.
// This function is called by [testify Suite].
// Note: Having initialization code in this function allows `NewSuite` to not
// return an error in order to write a single line for
// `suite.Run(t, &vmSuite{Suite: e2e.NewSuite(...)})`
func (suite *Suite[Env]) SetupSuite() {
	require := require.New(suite.T())

	// Check if the Env type is correct otherwise raises an error before creating the env.
	err := client.CheckEnvStructValid[Env]()
	require.NoError(err)

	env, _, upResult, err := createEnv(suite, suite.stackDef)
	require.NoError(err)

	suite.Env = env
	err = client.CallStackInitializers(&suite.auth, env, upResult)
	require.NoError(err)
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

func createEnv[Env any](suite *Suite[Env], stackDef *StackDefinition[Env]) (*Env, *auto.Stack, auto.UpResult, error) {
	var env *Env
	ctx := context.Background()

	stack, stackOutput, err := infra.GetStackManager().GetStack(
		ctx,
		suite.stackName,
		suite.stackDef.ConfigMap,
		func(ctx *pulumi.Context) error {
			var err error
			env, err = stackDef.EnvFactory(ctx)
			return err
		}, false)

	return env, stack, stackOutput, err
}
