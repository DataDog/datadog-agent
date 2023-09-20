// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides the API to manage environments and organize E2E tests.
//
// Here is a small example of E2E tests.
// E2E tests use [testify Suite] and it is strongly recommended to read the documentation of
// [testify Suite] if you are not familiar with it.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//	)
//
//	type vmSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//
//	func TestVMSuite(t *testing.T) {
//		e2e.Run[e2e.VMEnv](t, &vmSuite{}, e2e.EC2VMStackDef())
//	}
//
//	func (v *vmSuite) TestBasicVM() {
//		v.Env().VM.Execute("ls")
//	}
//
// To write an E2E test:
//
// 1. Define your own [suite] type with the embedded [e2e.Suite] struct.
//
//	type vmSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//
// [e2e.VMEnv] defines the components available in your stack. See "Using existing stack definition" section for more information.
//
// 2. Write a regular Go test function that runs the test suite using [e2e.Run].
//
//	func TestVMSuite(t *testing.T) {
//		e2e.Run[e2e.VMEnv](t, &vmSuite{}, e2e.EC2VMStackDef())
//	}
//
// The first argument of [e2e.Run] is an instance of type [*testing.T].
//
// The second argument is a pointer to an empty instance of the previous defined structure (&vmSuite{} in our example)
//
// The third parameter defines the environment. See "Using existing stack definition" section for more information about a environment definition.
//
// 3. Write a test function
//
//	func (v *vmSuite) TestBasicVM() {
//		v.Env().VM.Execute("ls")
//	}
//
// [e2e.Suite.Env] gives access to the components in your environment.
//
// Depending on your stack definition, [e2e.Suite.Env] can provide the following objects:
//   - [client.VM]: A virtual machine where you can execute commands.
//   - [client.Agent]: A struct that provides methods to run datadog agent commands.
//   - [client.Fakeintake]: A struct that provides methods to run queries to a fake instance of Datadog intake.
//
// # Using an existing stack definition
//
// The stack definition defines the components available in your environment.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//	)
//
//	type vmSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//
//	func TestVMSuite(t *testing.T) {
//		e2e.Run[e2e.VMEnv](t, &vmSuite{}, e2e.EC2VMStackDef())
//	}
//
// In this example, the components available are defined by the struct [e2e.VMEnv] which contains a virtual machine.
// The generic type of [e2e.Suite] must match the type of the stack definition.
// In our example, [e2e.EC2VMStackDef] returns an instance of [*e2e.StackDefinition][[e2e.VMEnv]].
//
// # e2e.EC2VMStackDef
//
// [e2e.EC2VMStackDef] creates an environment with a virtual machine.
// The available options are located in the [ec2params] package.
//
//	import (
//		"testing"
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//		"github.com/DataDog/test-infra-definitions/components/os"
//		"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
//		"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
//	)
//
//	type vmSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//
//	func TestVMSuite(t *testing.T) {
//		e2e.Run[e2e.VMEnv](t, &vmSuite{}, e2e.EC2VMStackDef(
//			ec2params.WithImageName("ami-0a0c8eebcdd6dcbd0", os.ARM64Arch, ec2os.UbuntuOS),
//			ec2params.WithName("My-instance"),
//		))
//	}
//
//	func (v *vmSuite) TestBasicVM() {
//		v.Env().VM.Execute("ls")
//	}
//
// # e2e.AgentStackDef
//
// [e2e.AgentStackDef] creates an environment with an Agent installed on a virtual machine.
// The available options are located in the [agentparams] package.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//		"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
//		"github.com/DataDog/test-infra-definitions/components/os"
//		"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
//		"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
//		"github.com/stretchr/testify/require"
//	)
//
//	type agentSuite struct {
//		e2e.Suite[e2e.AgentEnv]
//	}
//
//	func TestVMSuite(t *testing.T) {
//		e2e.Run[e2e.AgentEnv](t, &agentSuite{}, e2e.AgentStackDef(
//			[]ec2params.Option{
//				ec2params.WithImageName("ami-0a0c8eebcdd6dcbd0", os.ARM64Arch, ec2os.UbuntuOS),
//				ec2params.WithName("My-instance"),
//			},
//			agentparams.WithAgentConfig("log_level: debug"),
//			agentparams.WithTelemetry(),
//		))
//	}
//
//	func (v *agentSuite) TestBasicAgent() {
//		config := v.Env().Agent.Config()
//		require.Contains(v.T(), config, "log_level: debug")
//	}
//
// # Defining your stack definition
//
// In some special cases, you have to define a custom environment.
// Here is an example of an environment with Docker installed on a virtual machine.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
//		"github.com/DataDog/test-infra-definitions/components/datadog/agent/docker"
//		"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
//		"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
//	)
//
//	type dockerSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//
//	func TestDockerSuite(t *testing.T) {
//		e2e.Run[e2e.VMEnv](t, &dockerSuite{}, e2e.EnvFactoryStackDef(dockerEnvFactory))
//	}
//
//	func dockerEnvFactory(ctx *pulumi.Context) (*e2e.VMEnv, error) {
//		vm, err := ec2vm.NewUnixEc2VM(ctx)
//		if err != nil {
//			return nil, err
//		}
//
//		_, err = docker.NewAgentDockerInstaller(vm.UnixVM, docker.WithAgent(docker.WithAgentImageTag("7.42.0")))
//
//		if err != nil {
//			return nil, err
//		}
//
//		return &e2e.VMEnv{
//			VM: client.NewVM(vm),
//		}, nil
//	}
//
//	func (docker *dockerSuite) TestDocker() {
//		docker.Env().VM.Execute("docker container ls")
//	}
//
// [e2e.EnvFactoryStackDef] is used to define a custom environment.
// Here is a non exhaustive list of components that can be used to create a custom environment:
//   - [EC2 VM]: Provide methods to create a virtual machine on EC2.
//   - [Agent]: Provide methods to install the Agent on a virtual machine
//   - [File Manager]: Provide methods to manipulate files and folders
//
// # Organizing your tests
//
// The execution order for tests in [testify Suite] is IMPLEMENTATION SPECIFIC
// UNLIKE REGULAR GO TESTS.
//
// # Having a single environment
//
// In the simple case, there is a single environment and each test checks one specific thing.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//	)
//
//	type singleEnvSuite struct {
//		e2e.Suite[e2e.AgentEnv]
//	}
//
//	func TestSingleEnvSuite(t *testing.T) {
//		e2e.Run[e2e.AgentEnv](t, &singleEnvSuite{}, e2e.AgentStackDef())
//	}
//
//	func (suite *singleEnvSuite) Test1() {
//		// Check feature 1
//	}
//
//	func (suite *singleEnvSuite) Test2() {
//		// Check feature 2
//	}
//
//	func (suite *singleEnvSuite) Test3() {
//		// Check feature 3
//	}
//
// # Having different environments
//
// In this scenario, the environment is different for each test (or for most of them).
// [e2e.Suite.UpdateEnv] is used to update the environment.
// Keep in mind that using [e2e.Suite.UpdateEnv] to update virtual machine settings can destroy
// the current virtual machine and create a new one when updating the operating system for example.
//
// Note: Calling twice [e2e.Suite.UpdateEnv] with the same argument does nothing.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//		"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
//		"github.com/stretchr/testify/require"
//	)
//
//	type multipleEnvSuite struct {
//		e2e.Suite[e2e.AgentEnv]
//	}
//
//	func TestMultipleEnvSuite(t *testing.T) {
//		e2e.Run[e2e.AgentEnv](t, &multipleEnvSuite{}, e2e.AgentStackDef())
//	}
//
//	func (suite *multipleEnvSuite) TestLogDebug() {
//		suite.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("log_level: debug"))))
//		config := suite.Env().Agent.Config()
//		require.Contains(suite.T(), config, "log_level: debug")
//	}
//
//	func (suite *multipleEnvSuite) TestLogInfo() {
//		suite.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("log_level: info"))))
//		config := suite.Env().Agent.Config()
//		require.Contains(suite.T(), config, "log_level: info")
//	}
//
// # Having few environments
//
// You may sometime have few environments but several tests for each on them.
// You can still use [e2e.Suite.UpdateEnv] as explained in the previous section but using
// [Subtests] is an alternative solution.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//		"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
//	)
//
//	type subTestSuite struct {
//		e2e.Suite[e2e.AgentEnv]
//	}
//
//	func TestSubTestSuite(t *testing.T) {
//		e2e.Run[e2e.AgentEnv](t, &subTestSuite{}, e2e.AgentStackDef())
//	}
//
//	func (suite *subTestSuite) TestLogDebug() {
//		suite.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("log_level: debug"))))
//		suite.T().Run("MySubTest1", func(t *testing.T) {
//			// Sub test 1
//		})
//		suite.T().Run("MySubTest2", func(t *testing.T) {
//			// Sub test 2
//		})
//	}
//
//	func (suite *subTestSuite) TestLogInfo() {
//		suite.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("log_level: info"))))
//		suite.T().Run("MySubTest1", func(t *testing.T) {
//			// Sub test 1
//		})
//		suite.T().Run("MySubTest2", func(t *testing.T) {
//			// Sub test 2
//		})
//	}
//
// # WithDevMode
//
// When writing a new e2e test, it is important to iterate quickly until your test succeeds.
// You can use [params.WithDevMode] to not destroy the environment when the test finishes.
// For example it allows you to not create a new virtual machine each time you run a test.
// Note: [params.WithDevMode] is ignored when the test runs on the CI but it should be removed when you finish the writing of the test.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
//	)
//
//	type vmSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//
//	func TestVMSuite(t *testing.T) {
//		e2e.Run[e2e.VMEnv](t, &vmSuite{}, e2e.EC2VMStackDef(), params.WithDevMode())
//	}
//
//	func (v *vmSuite) TestBasicVM() {
//		v.Env().VM.Execute("ls")
//	}
//
// [Subtests]: https://go.dev/blog/subtests
// [suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
// [File Manager]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/command#FileManager
// [EC2 VM]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/scenarios/aws/vm/ec2VM
// [Agent]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
// [ec2params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/scenarios/aws/vm/ec2params
// [agentparams]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agentparams
package e2e

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
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

	params          params.Params
	defaultStackDef *StackDefinition[Env]
	currentStackDef *StackDefinition[Env]
	firstFailTest   string

	// These fields are initialized in SetupSuite
	env *Env

	isUpdateEnvCalledInThisTest bool
}

type suiteConstraint[Env any] interface {
	suite.TestingSuite
	initSuite(stackName string, stackDef *StackDefinition[Env], options ...params.Option)
}

// Run runs the tests defined in e2eSuite
//
// t is an instance of type [*testing.T].
//
// e2eSuite is a pointer to a structure with a [e2e.Suite] embbeded struct.
//
// stackDef defines the stack definition.
//
// options is an optional list of options like [DevMode], [SkipDeleteOnFailure] or [WithStackName].
//
//	type vmSuite struct {
//		e2e.Suite[e2e.VMEnv]
//	}
//	// ...
//	e2e.Run(t, &vmSuite{}, e2e.EC2VMStackDef())
func Run[Env any, T suiteConstraint[Env]](t *testing.T, e2eSuite T, stackDef *StackDefinition[Env], options ...params.Option) {
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

func (suite *Suite[Env]) initSuite(stackName string, stackDef *StackDefinition[Env], options ...params.Option) {
	suite.params.StackName = stackName
	suite.defaultStackDef = stackDef
	for _, o := range options {
		o(&suite.params)
	}
}

// Env returns the current environment.
// In order to improve the efficiency, this function behaves as follow:
//   - It creates the default environment if no environment exists.
//   - It restores the default environment if [e2e.Suite.UpdateEnv] was not already called during this test.
//     This avoid having to restore the default environment for each test even if [suite.UpdateEnv] immedialy
//     overrides the environment.
func (suite *Suite[Env]) Env() *Env {
	if suite.env == nil || !suite.isUpdateEnvCalledInThisTest {
		suite.UpdateEnv(suite.defaultStackDef)
	}
	return suite.env
}

// BeforeTest is executed right before the test starts and receives the suite and test names as input.
// This function is called by [testify Suite].
//
// If you override BeforeTest in your custom test suite type, the function must call [e2e.Suite.BeforeTest].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite[Env]) BeforeTest(suiteName, testName string) {
	suite.isUpdateEnvCalledInThisTest = false
}

// AfterTest is executed right after the test finishes and receives the suite and test names as input.
// This function is called by [testify Suite].
//
// If you override AfterTest in your custom test suite type, the function must call [e2e.Suite.AfterTest].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
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
//
// If you override SetupSuite in your custom test suite type, the function must call [e2e.Suite.SetupSuite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite[Env]) SetupSuite() {
	skipDelete, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.SkipDeleteOnFailure, false)
	if skipDelete {
		suite.params.SkipDeleteOnFailure = true
	}

	suite.Require().NotEmptyf(suite.params.StackName, "The stack name is empty. You must define it with WithName")
	// Check if the Env type is correct otherwise raises an error before creating the env.
	err := client.CheckEnvStructValid[Env]()
	suite.Require().NoError(err)
}

// TearDownSuite run after all the tests in the suite have been run.
// This function is called by [testify Suite].
//
// If you override TearDownSuite in your custom test suite type, the function must call [e2e.Suite.TearDownSuite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite[Env]) TearDownSuite() {
	if runner.GetProfile().AllowDevMode() && suite.params.DevMode {
		return
	}

	if suite.firstFailTest != "" && suite.params.SkipDeleteOnFailure {
		suite.Require().FailNow(fmt.Sprintf("%v failed. As SkipDeleteOnFailure feature is enabled the tests after %v were skipped. "+
			"The environment of %v was kept.", suite.firstFailTest, suite.firstFailTest, suite.firstFailTest))
		return
	}

	// TODO: Implement retry on delete
	ctx, cancel := context.WithTimeout(context.Background(), deleteTimeout)
	defer cancel()
	err := infra.GetStackManager().DeleteStack(ctx, suite.params.StackName)
	if err != nil {
		suite.T().Errorf("unable to delete stack: %s, err :%v", suite.params.StackName, err)
		suite.T().Fail()
	}
}

func createEnv[Env any](suite *Suite[Env], stackDef *StackDefinition[Env]) (*Env, auto.UpResult, error) {
	var env *Env
	ctx := context.Background()

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(
		ctx,
		suite.params.StackName,
		stackDef.configMap,
		func(ctx *pulumi.Context) error {
			var err error
			env, err = stackDef.envFactory(ctx)
			return err
		}, false)

	return env, stackOutput, err
}

// UpdateEnv updates the environment.
// This affects only the test that calls this function.
// Test functions that don't call UpdateEnv have the environment defined by [e2e.Run].
func (suite *Suite[Env]) UpdateEnv(stackDef *StackDefinition[Env]) {
	if stackDef != suite.currentStackDef {
		if (suite.firstFailTest != "" || suite.T().Failed()) && suite.params.SkipDeleteOnFailure {
			// In case of failure, do not override the environment
			suite.T().SkipNow()
		}
		env, upResult, err := createEnv(suite, stackDef)
		suite.Require().NoError(err)
		err = client.CallStackInitializers(suite.T(), env, upResult)
		suite.Require().NoError(err)
		suite.env = env
		suite.currentStackDef = stackDef
	}
	suite.isUpdateEnvCalledInThisTest = true
}
