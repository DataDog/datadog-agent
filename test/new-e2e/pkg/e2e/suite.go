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
//		v.Env().VM.MustExecute("ls")
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
//		v.Env().VM.MustExecute("ls")
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
//		v.Env().VM.MustExecute("ls")
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
//		docker.Env().VM.MustExecute("docker container ls")
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
//		v.Env().VM.MustExecute("ls")
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
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"

	"github.com/stretchr/testify/suite"
)

const (
	importKey = "import"

	createTimeout          = 60 * time.Minute
	deleteTimeout          = 30 * time.Minute
	provisionerGracePeriod = 2 * time.Second
)

// Suite is a generic inteface used internally, only implemented by BaseSuite
type Suite[Env any] interface {
	suite.TestingSuite

	init(params []SuiteOption, self Suite[Env])

	UpdateEnv(...Provisioner)
	Env() *Env
}

var _ Suite[any] = &BaseSuite[any]{}

type BaseSuite[Env any] struct {
	suite.Suite

	env    *Env
	params suiteParams

	currentProvisioners map[string]Provisioner
	targetProvisioners  map[string]Provisioner

	firstFailTest string
}

// Custom methods
func (bs *BaseSuite[Env]) Env() *Env {
	bs.reconcileEnv()
	return bs.env
}

func (bs *BaseSuite[Env]) UpdateEnv(newProvisioners ...Provisioner) {
	uniqueIDs := make(map[string]struct{})
	newTargetProvisioners := make(map[string]Provisioner, len(newProvisioners))
	for _, provisioner := range newProvisioners {
		if _, found := uniqueIDs[provisioner.ID()]; found {
			bs.T().Errorf("Multiple providers with same id found, provisioner with id %s already exists", provisioner.ID())
			bs.T().FailNow()
		}

		uniqueIDs[provisioner.ID()] = struct{}{}
		newTargetProvisioners[provisioner.ID()] = provisioner
	}

	bs.targetProvisioners = newTargetProvisioners
	bs.reconcileEnv()
}

func (bs *BaseSuite[Env]) init(options []SuiteOption, self Suite[Env]) {
	for _, o := range options {
		o(&bs.params)
	}

	if bs.params.devMode && !runner.GetProfile().AllowDevMode() {
		bs.params.devMode = false
	}

	if !bs.params.skipDeleteOnFailure {
		bs.params.skipDeleteOnFailure, _ = runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.SkipDeleteOnFailure, false)
	}

	if bs.params.stackName == "" {
		sType := reflect.TypeOf(self).Elem()
		hash := utils.StrHash(sType.PkgPath()) // hash of PkgPath in order to have a unique stack name
		bs.params.stackName = fmt.Sprintf("e2e-%s-%s", sType.Name(), hash)
	}
}

func (bs *BaseSuite[Env]) reconcileEnv() {
	if reflect.DeepEqual(bs.currentProvisioners, bs.targetProvisioners) {
		return
	}

	ctx, cancel := bs.providerContext(createTimeout)
	defer cancel()

	newEnv, newEnvFields, newEnvValues, err := bs.createEnv()
	if err != nil {
		panic(fmt.Errorf("unable to create new env: %T for stack: %s, err: %v", newEnv, bs.params.stackName, err))
	}

	resources := make(RawResources)
	for id, provisioner := range bs.targetProvisioners {
		var provisionerResources RawResources
		var err error

		switch pType := provisioner.(type) {
		case TypedProvisioner[Env]:
			provisionerResources, err = pType.ProvisionEnv(bs.params.stackName, ctx, newEnv)
		case UntypedProvisioner:
			provisionerResources, err = pType.Provision(bs.params.stackName, ctx)
		default:
			panic(fmt.Errorf("provisioner of type %T does not implement UntypedProvisioner nor TypedProvisioner", provisioner))
		}

		if err != nil {
			panic(fmt.Errorf("unable to provision stack: %s, provisioner %s, err: %v", bs.params.stackName, id, err))
		}

		resources.Merge(provisionerResources)
	}

	// Env is taken as parameter as some fields may have keys set by Env pulumi program.
	err = bs.buildEnvFromResources(resources, newEnvFields, newEnvValues, newEnv)
	if err != nil {
		panic(fmt.Errorf("unable to build env: %T from resources for stack: %s, err: %v", newEnv, bs.params.stackName, err))
	}

	// If env implements Initializable, we call Init
	if initializable, ok := any(newEnv).(Initializable); ok {
		if err := initializable.Init(bs); err != nil {
			panic(fmt.Errorf("failed to init environment, err: %v", err))
		}
	}

	// On success we update the current environment
	bs.env = newEnv
	bs.currentProvisioners = bs.targetProvisioners
}

func (bs *BaseSuite[Env]) createEnv() (*Env, []reflect.StructField, []reflect.Value, error) {
	var env Env
	envFields := reflect.VisibleFields(reflect.TypeOf(&env).Elem())
	envValue := reflect.ValueOf(&env)

	retainedFields := make([]reflect.StructField, 0)
	retainedValues := make([]reflect.Value, 0)
	for _, field := range envFields {
		if !field.IsExported() {
			continue
		}

		importKeyFromTag := field.Tag.Get(importKey)
		isImportable := field.Type.Implements(reflect.TypeOf((*components.Importable)(nil)).Elem())
		isPtrImportable := reflect.PtrTo(field.Type).Implements(reflect.TypeOf((*components.Importable)(nil)).Elem())

		// Produce meaningful error in case we have an importKey but field is not importable
		if importKeyFromTag != "" && !isImportable {
			return nil, nil, nil, fmt.Errorf("resource named %s has %s key but does not implement Importable interface", field.Name, importKey)
		}

		if !isImportable && isPtrImportable {
			return nil, nil, nil, fmt.Errorf("resource named %s of type %T implements Importable on pointer receiver but is not a pointer", field.Name, field.Type)
		}

		if !isImportable {
			continue
		}

		// Create zero-value if not created (pointer to struct)
		fieldValue := envValue.Elem().FieldByIndex(field.Index)
		if fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}

		retainedFields = append(retainedFields, field)
		retainedValues = append(retainedValues, fieldValue)
	}

	return &env, retainedFields, retainedValues, nil
}

func (bs *BaseSuite[Env]) buildEnvFromResources(resources RawResources, fields []reflect.StructField, values []reflect.Value, env *Env) error {
	if len(fields) != len(values) {
		panic("fields and values must have the same length")
	}

	if len(resources) == 0 {
		return nil
	}

	for idx, fieldValue := range values {
		field := fields[idx]
		importKeyFromTag := field.Tag.Get(importKey)

		// If a field value is nil, it means that it was explicitly set to nil by provisioners, hence not available
		// We should not find it in the resources map, returning an error in this case.
		if fieldValue.IsNil() {
			if _, found := resources[importKeyFromTag]; found {
				return fmt.Errorf("resource named %s has key %s but is nil", fields[idx].Name, importKeyFromTag)
			} else {
				continue
			}
		}

		importable := fieldValue.Interface().(components.Importable)
		resourceKey := importable.Key()
		if importKeyFromTag != "" {
			resourceKey = importKeyFromTag
		}
		if resourceKey == "" {
			return fmt.Errorf("resource named %s has no import key set and no annotation", field.Name)
		}

		if rawResource, found := resources[resourceKey]; found {
			err := importable.Import(rawResource, fieldValue.Interface())
			if err != nil {
				return fmt.Errorf("failed to import resource named: %s with key: %s, err: %w", field.Name, resourceKey, err)
			}

			// See if the component requires init
			if initializable, ok := fieldValue.Interface().(Initializable); ok {
				if err := initializable.Init(bs); err != nil {
					return fmt.Errorf("failed to init resource named: %s with key: %s, err: %w", field.Name, resourceKey, err)
				}
			}
		} else {
			return fmt.Errorf("expected resource named: %s with key: %s but not returned by provisioners", field.Name, resourceKey)
		}
	}

	return nil
}

func (bs *BaseSuite[Env]) providerContext(opTimeout time.Duration) (context.Context, context.CancelFunc) {
	var ctx context.Context
	var cancel func()

	if deadline, ok := bs.T().Deadline(); ok {
		deadline = deadline.Add(-provisionerGracePeriod)
		ctx, cancel = context.WithDeadlineCause(context.Background(), deadline, errors.New("go test timeout almost reached, cancelling provisioners"))
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), opTimeout)
	}

	return ctx, cancel
}

//
// Overriden methods
//

// BeforeTest is executed right before the test starts and receives the suite and test names as input.
// This function is called by [testify Suite].
//
// If you override BeforeTest in your custom test suite type, the function must call [test.BaseSuite.BeforeTest].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (bs *BaseSuite[Env]) BeforeTest(string, string) {
	// Reset provisioners to original provisioners
	bs.targetProvisioners = bs.params.provisioners
}

// AfterTest is executed right after the test finishes and receives the suite and test names as input.
// This function is called by [testify Suite].
//
// If you override AfterTest in your custom test suite type, the function must call [test.BaseSuite.AfterTest].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (bs *BaseSuite[Env]) AfterTest(suiteName, testName string) {
	if bs.T().Failed() && bs.firstFailTest == "" {
		// As far as I know, there is no way to prevent other tests from being
		// run when a test fail. Even calling panic doesn't work.
		// Instead, this code stores the name of the first fail test and prevents
		// the environment to be updated.
		// Note: using os.Exit(1) prevents other tests from being run but at the
		// price of having no test output at all.
		bs.firstFailTest = fmt.Sprintf("%v.%v", suiteName, testName)
	}
}

// TearDownSuite run after all the tests in the suite have been run.
// This function is called by [testify Suite].
//
// If you override TearDownSuite in your custom test suite type, the function must call [e2e.Suite.TearDownSuite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (bs *BaseSuite[Env]) TearDownSuite() {
	if bs.params.devMode {
		return
	}

	if bs.firstFailTest != "" && bs.params.skipDeleteOnFailure {
		bs.Require().FailNow(fmt.Sprintf("%v failed. As SkipDeleteOnFailure feature is enabled the tests after %v were skipped. "+
			"The environment of %v was kept.", bs.firstFailTest, bs.firstFailTest, bs.firstFailTest))
		return
	}

	ctx, cancel := bs.providerContext(deleteTimeout)
	defer cancel()

	atLeastOneFailure := false
	for id, provisioner := range bs.currentProvisioners {
		if err := provisioner.Delete(bs.params.stackName, ctx); err != nil {
			bs.T().Errorf("unable to delete stack: %s, provisioner %s, err: %v", bs.params.stackName, id, err)
			atLeastOneFailure = true
		}
	}

	if atLeastOneFailure {
		bs.T().Fail()
	}
}

// Unfortunatly, we cannot use `s Suite[Env]` as Go is not able to match it with a struct
// However it's able to verify the same constraint on T
func Run[Env any, T Suite[Env]](t *testing.T, s T, options ...SuiteOption) {
	options = append(options, WithDevMode())
	s.init(options, s)
	suite.Run(t, s)
}
