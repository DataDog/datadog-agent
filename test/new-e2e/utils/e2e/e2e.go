// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides tools to manage environments and run E2E tests.
// See [Suite] for an example of the usage.
package e2e

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/suite"
)

const (
	deleteTimeout = 30 * time.Minute
)

// Suite manages the environment creation and runs E2E tests.
type Suite[Env any] struct {
	suite.Suite

	stackName       string
	defaultStackDef *StackDefinition[Env]
	currentStackDef *StackDefinition[Env]
	firstFailTest   string

	// These fields are initialized in SetupSuite
	env  *Env
	auth client.Authentification

	isUpdateEnvCalledInThisTest bool

	// Setting devMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	devMode bool

	skipDeleteOnFailure bool
}

type suiteConstraint[Env any] interface {
	suite.TestingSuite
	initSuite(stackName string, stackDef StackDefinition[Env], options ...func(*Suite[Env]))
}

func Run[Env any, T suiteConstraint[Env]](t *testing.T, e2eSuite T, stackDef StackDefinition[Env], options ...func(*Suite[Env])) {
	suiteType := reflect.TypeOf(e2eSuite).Elem()
	name := suiteType.Name()
	pkgPaths := suiteType.PkgPath()
	pkgs := strings.Split(pkgPaths, "/")

	// Use the hash of PkgPath in order to have a uniq stack name
	hash := utils.StrHash(pkgs...)

	// Example: "e2e-e2eSuite-cbb731954db42b"
	defaultStackName := fmt.Sprintf("%v-%v-%v", pkgs[len(pkgs)-1], name, hash)

	e2eSuite.initSuite(defaultStackName, stackDef, options...)
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
	suite.defaultStackDef = stackDef
	for _, o := range options {
		o(suite)
	}
}

// WithStackName overrides the stack name.
// This function is useful only when using e2e.Run.
func WithStackName[Env any](stackName string) func(*Suite[Env]) {
	return func(suite *Suite[Env]) {
		suite.stackName = stackName
	}
}

func DevMode[Env any]() func(*Suite[Env]) {
	return func(suite *Suite[Env]) {
		suite.devMode = true
	}
}

func SkipDeleteOnFailure[Env any]() func(*Suite[Env]) {
	return func(suite *Suite[Env]) {
		suite.skipDeleteOnFailure = true
	}
}

// Env returns the current environment.
// In order to improve the efficiency, this function behaves as follow:
//   - It creates the default environment if no environment exists. It happens only during the first call of the test suite.
//   - It restores the default environment if UpdateEnv was not already called during this test.
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

func (suite *Suite[Env]) AfterTest(suiteName, testName string) {
	if suite.T().Failed() && suite.firstFailTest == "" {
		// As far as I know, there is no way to prevent other tests from being
		// run when a test fail. Even calling panic doesn't work.
		// Instead, this code stores the name of the first fail test and prevents
		// the environment to be updated.
		// Note: using os.Exit(1) prevents other tests from being run but at the
		// price of having no test output at all.
		suite.firstFailTest = fmt.Sprintf("%v.%v", suiteName, testName)
	}
}

// SetupSuite method will run before the tests in the suite are run.
// This function is called by [testify Suite].
// Note: Having initialization code in this function allows `NewSuite` to not
// return an error in order to write a single line for
// `suite.Run(t, &vmSuite{Suite: e2e.NewSuite(...)})`
func (suite *Suite[Env]) SetupSuite() {
	skipDelete, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.SkipDeleteOnFailure, false)
	if skipDelete {
		suite.skipDeleteOnFailure = true
	}

	suite.Require().NotEmptyf(suite.stackName, "The stack name is empty. You must define it with WithName")
	// Check if the Env type is correct otherwise raises an error before creating the env.
	err := client.CheckEnvStructValid[Env]()
	suite.Require().NoError(err)
}

// TearDownTestSuite run after all the tests in the suite have been run.
// This function is called by [testify Suite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite[Env]) TearDownSuite() {
	if runner.GetProfile().AllowDevMode() && suite.devMode {
		return
	}

	if suite.firstFailTest != "" && suite.skipDeleteOnFailure {
		suite.Require().FailNow(fmt.Sprintf("%v failed. As SkipDeleteOnFailure feature is enabled the tests after %v were skipped. "+
			"The environment of %v was kept.", suite.firstFailTest, suite.firstFailTest, suite.firstFailTest))
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
		stackDef.configMap,
		func(ctx *pulumi.Context) error {
			var err error
			env, err = stackDef.envFactory(ctx)
			return err
		}, false)

	return env, stackOutput, err
}

func (suite *Suite[Env]) UpdateEnv(stackDef *StackDefinition[Env]) {
	if stackDef != suite.currentStackDef {
		if (suite.firstFailTest != "" || suite.T().Failed()) && suite.skipDeleteOnFailure {
			// In case of failure, do not override the environment
			suite.T().SkipNow()
		}
		env, upResult, err := createEnv(suite, stackDef)
		suite.Require().NoError(err)
		err = client.CallStackInitializers(&suite.auth, env, upResult)
		suite.Require().NoError(err)
		suite.env = env
		suite.currentStackDef = stackDef
	}
	suite.isUpdateEnvCalledInThisTest = true
}
