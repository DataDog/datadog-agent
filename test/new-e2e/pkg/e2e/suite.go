// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides the API to manage environments and organize E2E tests.
// Three major concepts are used to write E2E tests:
//   - [e2e.Provisioner]: A provisioner is a component that provide compute resources (usually Cloud resources). Most common is Pulumi through `test-infra-definitions`.
//   - [e2e.BaseSuite]: A TestSuite is a collection of tests that share the ~same environment.
//   - Environment: An environment is a collection of resources (virtual machine, agent, etc). An environment is filled by provisioners.
//
// See usage examples in the [examples] package.
//
// # Provisioners
//
// Three provisioners are available:
//   - [e2e.PulumiProvisioner]: A provisioner that uses Pulumi to create resources.
//
// Pulumi Provisioner can be typed or untyped:
//   - Typed provisioners are provisioners that are typed with the environment they provision and the `Run` function must be defined in `datadog-agent` inline.
//   - Untyped provisioners are provisioners that are not typed with the environment they provision and the `Run` function can come from anywhere.
//   - [e2e.StaticProvisioner]: A provisioner that uses static resources from a JSON file. The static provisioner is Untyped.
//
// # Impact of Typed vs Untyped provisioners
// Typed provisioners are more convenient to use as they are typed with the environment they provision, however they do require a close mapping between the RunFunc and the environment.
// With a Typed provisioner, the `component.Export()` function is used to match an Environment field with a Pulumi resource.
//
// An Untyped provisioner is more flexible as it does not require a close mapping between the RunFunc and the environment. It allows to get resources from anywhere in the same environment.
// However it means that the environment needs to be annotated with the `import` tag to match the resource key. See for instance the [examples/suite_serial_kube_test.go] file.
//
// # Out-of-the-box environments and provisioners
//
// Check the [environments] package for a list of out-of-the-box environments, for instance [environments.VM].
// Check the `environments/<cloud>` for a list of out-of-the-box provisioners, for instance [environments/aws/vm].
//
// # The BaseSuite test suite
//
// The [e2e.BaseSuite] test suite is a [testify Suite] that wraps environment and provisioners.
// It allows to easily write tests that share the same environment without having to re-implement boilerplate code.
// Check all the [e2e.SuiteOption] to customize the behavior of the BaseSuite.
//
// Note: By default, the BaseSuite test suite will delete the environment when the test suite finishes (whether it's successful or not).
// During development, it's highly recommended to use the [params.WithDevMode] option to prevent the environment from being deleted.
// [params.WithDevMode] is automatically enabled when the `E2E_DEV_MODE` environment variable is set to `true`.
//
// # Organizing your tests
//
// The execution order for tests in [testify Suite] is IMPLEMENTATION SPECIFIC
// UNLIKE REGULAR GO TESTS.
// Use subtests for ordered tests and environments update.
//
// # Having a single environment
//
// In the simple case, there is a single environment and each test checks one specific thing.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
//	)
//
//	type singleEnvSuite struct {
//		e2e.BaseSuite[environments.VM]
//	}
//
//	func TestSingleEnvSuite(t *testing.T) {
//		e2e.Run(t, &singleEnvSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
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
// You may sometime have different environments but several tests for each on them.
// You can use [e2e.Suite.UpdateEnv] to do that. Using `UpdateEnv` between groups of [Subtests].
// Note that between `TestLogDebug` and `TestLogInfo`, the environment is reverted to the original one.
//
//	import (
//		"testing"
//
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
//		"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
//		awsvm "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/vm"
//		"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
//	)
//
//	type subTestSuite struct {
//		e2e.Suite[environments.VM]
//	}
//
//	func TestSubTestSuite(t *testing.T) {
//		e2e.Run(t, &singleEnvSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
//	}
//
//	func (suite *subTestSuite) TestLogDebug() {
//		// First group of subsets
//		suite.T().Run("MySubTest1", func(t *testing.T) {
//			// Sub test 1
//		})
//		suite.T().Run("MySubTest2", func(t *testing.T) {
//			// Sub test 2
//		})
//
//		v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: debug"))))
//
//		// Second group of subsets
//		suite.T().Run("MySubTest3", func(t *testing.T) {
//			// Sub test 3
//		})
//	}
//
//	func (suite *subTestSuite) TestLogInfo() {
//		// First group of subsets
//		suite.T().Run("MySubTest1", func(t *testing.T) {
//			// Sub test 1
//		})
//		suite.T().Run("MySubTest2", func(t *testing.T) {
//			// Sub test 2
//		})
//
//		v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: info"))))
//
//		// Second group of subsets
//		suite.T().Run("MySubTest3", func(t *testing.T) {
//			// Sub test 3
//		})
//	}
//
// [Subtests]: https://go.dev/blog/subtests
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
package e2e

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"

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

// BaseSuite is a generic test suite that wraps testify.Suite
type BaseSuite[Env any] struct {
	suite.Suite

	env           *Env
	datadogClient *datadog.Client
	params        suiteParams

	originalProvisioners ProvisionerMap
	currentProvisioners  ProvisionerMap

	firstFailTest string
	startTime     time.Time
	endTime       time.Time
	initOnly      bool

	testSessionOutputDir     string
	onceTestSessionOutputDir sync.Once
}

//
// Custom methods
//

// Env returns the current environment
func (bs *BaseSuite[Env]) Env() *Env {
	return bs.env
}

// UpdateEnv updates the environment with new provisioners.
func (bs *BaseSuite[Env]) UpdateEnv(newProvisioners ...Provisioner) {
	uniqueIDs := make(map[string]struct{})
	targetProvisioners := make(ProvisionerMap, len(newProvisioners))
	for _, provisioner := range newProvisioners {
		if _, found := uniqueIDs[provisioner.ID()]; found {
			panic(fmt.Errorf("Multiple providers with same id found, provisioner with id %s already exists", provisioner.ID()))
		}

		uniqueIDs[provisioner.ID()] = struct{}{}
		targetProvisioners[provisioner.ID()] = provisioner
	}
	if err := bs.reconcileEnv(targetProvisioners); err != nil {
		panic(err)
	}
}

// IsDevMode returns true if the test suite is running in dev mode.
// WARNING: IsDevMode should not be used. It's a recipe to get tests working locally but failing in CI.
func (bs *BaseSuite[Env]) IsDevMode() bool {
	return bs.params.devMode
}

// StartTime returns the time when test suite started
func (bs *BaseSuite[Env]) StartTime() time.Time {
	return bs.startTime
}

// EndTime returns the time when test suite ended
func (bs *BaseSuite[Env]) EndTime() time.Time {
	return bs.endTime
}

// DatadogClient returns a Datadog client that can be used to send telemtry info to dddev during e2e tests
func (bs *BaseSuite[Env]) DatadogClient() *datadog.Client {
	return bs.datadogClient
}

func (bs *BaseSuite[Env]) init(options []SuiteOption, self Suite[Env]) {
	for _, o := range options {
		o(&bs.params)
	}

	initOnly, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.InitOnly, false)
	if err == nil {
		bs.initOnly = initOnly
	}

	if !runner.GetProfile().AllowDevMode() {
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

	bs.originalProvisioners = bs.params.provisioners
}

func (bs *BaseSuite[Env]) reconcileEnv(targetProvisioners ProvisionerMap) error {
	if reflect.DeepEqual(bs.currentProvisioners, targetProvisioners) {
		bs.T().Logf("No change in provisioners, skipping environment update")
		return nil
	}

	logger := newTestLogger(bs.T())
	ctx, cancel := bs.providerContext(createTimeout)
	defer cancel()

	newEnv, newEnvFields, newEnvValues, err := bs.createEnv()
	if err != nil {
		return fmt.Errorf("unable to create new env: %T for stack: %s, err: %v", newEnv, bs.params.stackName, err)
	}

	// Check for removed provisioners, we need to call delete on them first
	for id, provisioner := range bs.currentProvisioners {
		if _, found := targetProvisioners[id]; !found {
			if err := provisioner.Destroy(ctx, bs.params.stackName, logger); err != nil {
				return fmt.Errorf("unable to delete stack: %s, provisioner %s, err: %v", bs.params.stackName, id, err)
			}
		}
	}

	// Then we provision new resources
	resources := make(RawResources)
	for id, provisioner := range targetProvisioners {
		var provisionerResources RawResources
		var err error

		switch pType := provisioner.(type) {
		case TypedProvisioner[Env]:
			provisionerResources, err = pType.ProvisionEnv(ctx, bs.params.stackName, logger, newEnv)
		case UntypedProvisioner:
			provisionerResources, err = pType.Provision(ctx, bs.params.stackName, logger)
		default:
			return fmt.Errorf("provisioner of type %T does not implement UntypedProvisioner nor TypedProvisioner", provisioner)
		}

		if err != nil {
			if diagnosableProvisioner, ok := provisioner.(Diagnosable); ok {
				stackName, err := infra.GetStackManager().GetPulumiStackName(bs.params.stackName)
				if err != nil {
					bs.T().Logf("unable to get stack name for diagnose, err: %v", err)
				} else {
					diagnoseResult, diagnoseErr := diagnosableProvisioner.Diagnose(ctx, stackName)
					if diagnoseErr != nil {
						bs.T().Logf("WARNING: Diagnose failed: %v", diagnoseErr)
					} else if diagnoseResult != "" {
						bs.T().Logf("Diagnose result: %s", diagnoseResult)
					}
				}

			}
			return fmt.Errorf("your stack '%s' provisioning failed, check logs above. Provisioner was %s, failed with err: %v", bs.params.stackName, id, err)
		}

		resources.Merge(provisionerResources)
	}

	// When INIT_ONLY is set, we only partially provision the environment so we do not want initialize the environment
	if bs.initOnly {
		return nil
	}

	// Env is taken as parameter as some fields may have keys set by Env pulumi program.
	err = bs.buildEnvFromResources(resources, newEnvFields, newEnvValues)
	if err != nil {
		return fmt.Errorf("unable to build env: %T from resources for stack: %s, err: %v", newEnv, bs.params.stackName, err)
	}

	// If env implements Initializable, we call Init
	if initializable, ok := any(newEnv).(Initializable); ok {
		if err := initializable.Init(bs); err != nil {
			return fmt.Errorf("failed to init environment, err: %v", err)
		}
	}

	// On success we update the current environment
	// We need top copy provisioners to protect against external modifications
	bs.currentProvisioners = copyProvisioners(targetProvisioners)
	bs.env = newEnv
	return nil
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

func (bs *BaseSuite[Env]) buildEnvFromResources(resources RawResources, fields []reflect.StructField, values []reflect.Value) error {
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
			}

			continue
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
// Overridden methods
//

// SetupSuite run before all the tests in the suite have been run.
// This function is called by [testify Suite].
//
// If you override SetupSuite in your custom test suite type, the function must call [e2e.BaseSuite.SetupSuite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (bs *BaseSuite[Env]) SetupSuite() {
	bs.startTime = time.Now()
	// In `SetupSuite` we cannot fail as `TearDownSuite` will not be called otherwise.
	// Meaning that stack clean up may not be called.
	// We do implement an explicit recover to handle this manuallay.
	defer func() {
		err := recover()
		if err == nil {
			return
		}

		bs.T().Logf("Caught panic in SetupSuite, err: %v. Will try to TearDownSuite", err)
		bs.firstFailTest = "Initial provisioiningin SetupSuite" // This is required to handle skipDeleteOnFailure
		bs.TearDownSuite()

		// As we need to call `recover` to know if there was a panic, we wrap and forward the original panic to,
		// once again, stop the execution of the test suite.
		panic(fmt.Errorf("Forward panic in SetupSuite after TearDownSuite, err was: %v", err))
	}()

	// Setup Datadog Client to be used to send telemetry when writing e2e tests
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	bs.Require().NoError(err)
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	bs.Require().NoError(err)
	bs.datadogClient = datadog.NewClient(apiKey, appKey)

	if err := bs.reconcileEnv(bs.originalProvisioners); err != nil {
		// `panic()` is required to stop the execution of the test suite. Otherwise `testify.Suite` will keep on running suite tests.
		panic(err)
	}

	if bs.initOnly {
		bs.T().Skip("INIT_ONLY is set, skipping tests")
	}
}

// BeforeTest is executed right before the test starts and receives the suite and test names as input.
// This function is called by [testify Suite].
//
// If you override BeforeTest in your custom test suite type, the function must call [test.BaseSuite.BeforeTest].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (bs *BaseSuite[Env]) BeforeTest(string, string) {
	// Reset provisioners to original provisioners
	// In `Test` scope we can `panic`, it will be recovered and `AfterTest` will be called.
	// Next tests will be called as well
	if err := bs.reconcileEnv(bs.originalProvisioners); err != nil {
		panic(err)
	}
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
// If you override TearDownSuite in your custom test suite type, the function must call [e2e.BaseSuite.TearDownSuite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (bs *BaseSuite[Env]) TearDownSuite() {
	bs.endTime = time.Now()

	if bs.params.devMode {
		return
	}

	if bs.initOnly {
		bs.T().Logf("INIT_ONLY is set, skipping deletion")
		return
	}

	if bs.firstFailTest != "" && bs.params.skipDeleteOnFailure {
		bs.Require().FailNow(fmt.Sprintf("%v failed. As SkipDeleteOnFailure feature is enabled the tests after %v were skipped. "+
			"The environment of %v was kept.", bs.firstFailTest, bs.firstFailTest, bs.firstFailTest))
		return
	}

	ctx, cancel := bs.providerContext(deleteTimeout)
	defer cancel()

	for id, provisioner := range bs.originalProvisioners {
		// Run provisioner Diagnose before tearing down the stack
		if diagnosableProvisioner, ok := provisioner.(Diagnosable); ok {
			stackName, err := infra.GetStackManager().GetPulumiStackName(bs.params.stackName)
			if err != nil {
				bs.T().Logf("unable to get stack name for diagnose, err: %v", err)
			} else {
				diagnoseResult, diagnoseErr := diagnosableProvisioner.Diagnose(ctx, stackName)
				if diagnoseErr != nil {
					bs.T().Logf("WARNING: Diagnose failed: %v", diagnoseErr)
				} else if diagnoseResult != "" {
					bs.T().Logf("Diagnose result: %s", diagnoseResult)
				}
			}
		}

		if err := provisioner.Destroy(ctx, bs.params.stackName, newTestLogger(bs.T())); err != nil {
			bs.T().Errorf("unable to delete stack: %s, provisioner %s, err: %v", bs.params.stackName, id, err)
		}
	}
}

// GetRootOutputDir returns the root output directory for tests to store output files and artifacts.
// The directory is created on the first call to this function and reused in future calls.
//
// See BaseSuite.CreateTestOutputDir() for a function that returns a directory for the current test.
//
// See CreateRootOutputDir() for details on the root directory creation.
func (bs *BaseSuite[Env]) GetRootOutputDir() (string, error) {
	var err error
	bs.onceTestSessionOutputDir.Do(func() {
		// Store the timestamped directory to be used by all tests in the suite
		bs.testSessionOutputDir, err = CreateRootOutputDir()
	})
	return bs.testSessionOutputDir, err
}

// CreateTestOutputDir returns an output directory for the current test.
//
// See also CreateTestOutputDir()
func (bs *BaseSuite[Env]) CreateTestOutputDir() (string, error) {
	root, err := bs.GetRootOutputDir()
	if err != nil {
		return "", err
	}
	return CreateTestOutputDir(root, bs.T())
}

// Run is a helper function to run a test suite.
// Unfortunately, we cannot use `s Suite[Env]` as Go is not able to match it with a struct
// However it's able to verify the same constraint on T
func Run[Env any, T Suite[Env]](t *testing.T, s T, options ...SuiteOption) {
	devMode, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevMode, false)
	if err != nil {
		t.Logf("Unable to get DevMode value, DevMode will be disabled, error: %v", err)
	} else if devMode {
		options = append(options, WithDevMode())
	}

	s.init(options, s)
	suite.Run(t, s)
}
