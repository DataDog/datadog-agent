// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

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

	init(params []Option, self Suite[Env])

	UpdateEnv(map[string]Provisioner)
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

func (bs *BaseSuite[Env]) UpdateEnv(newProvisioners map[string]Provisioner) {
	bs.targetProvisioners = newProvisioners
}

func (bs *BaseSuite[Env]) init(options []Option, self Suite[Env]) {
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

	var newEnv Env
	resources := make(RawResources)
	for id, provisioner := range bs.targetProvisioners {
		var provisionerResources RawResources
		var err error

		switch pType := provisioner.(type) {
		case TypedProvisioner[Env]:
			bs.createEnv(nil, &newEnv) // creating zero value for pointers in struct
			provisionerResources, err = pType.ProvisionEnv(bs.params.stackName, ctx, &newEnv)
		case UntypedProvisioner:
			provisionerResources, err = pType.Provision(bs.params.stackName, ctx)
		default:
			bs.T().Errorf("provisioner of type %T does not implement UntypedProvisioner nor TypedProvisioner", provisioner)
			bs.T().FailNow()
		}

		if err != nil {
			bs.T().Errorf("unable to provision stack: %s, provisioner %s, err: %v", bs.params.stackName, id, err)
			bs.T().FailNow()
		}

		resources.Merge(provisionerResources)
	}

	// Env is taken as parameter as some fiels may have keys set by Env pulumi program.
	err := bs.createEnv(resources, &newEnv)
	if err != nil {
		bs.T().Errorf("unable to create env from stack: %s, err: %v", bs.params.stackName, err)
		bs.T().FailNow()
	}

	// On success we update the current environment
	bs.env = &newEnv
	bs.currentProvisioners = bs.targetProvisioners
}

func (bs *BaseSuite[Env]) createEnv(resources RawResources, env *Env) error {
	envFields := reflect.VisibleFields(reflect.TypeOf(env).Elem())
	envValue := reflect.ValueOf(env)

	for _, field := range envFields {
		if !field.IsExported() {
			continue
		}

		importKeyFromTag := field.Tag.Get(importKey)
		isImportable := field.Type.Implements(reflect.TypeOf((*components.Importable)(nil)).Elem())
		isPtrImportable := reflect.PtrTo(field.Type).Implements(reflect.TypeOf((*components.Importable)(nil)).Elem())

		// Produce meaningful error in case we have an importKey but field is not importable
		if importKeyFromTag != "" && !isImportable {
			return fmt.Errorf("resource named %s has %s key but does not implement Importable interface", field.Name, importKey)
		}

		if !isImportable && isPtrImportable {
			return fmt.Errorf("resource named %s of type %T implements Importable on pointer receiver but is not a pointer", field.Name, field.Type)
		}

		if !isImportable {
			continue
		}

		// Create zero-value if not created (pointer to struct)
		fieldValue := envValue.Elem().FieldByIndex(field.Index)
		if fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}

		// If resources == nil, we stop at creating zero value to be used in provisioner
		if resources != nil {
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
			} else {
				return fmt.Errorf("expected resource named: %s with key: %s but not returned by provisioners", field.Name, resourceKey)
			}
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
func Run[Env any, T Suite[Env]](t *testing.T, s T, options ...Option) {
	s.init(options, s)
	suite.Run(t, s)
}
