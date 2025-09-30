// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test || functionaltests || stresstests

package fxutil

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// NoDependencies defines a component which doesn't have any dependencies
type NoDependencies struct {
	fx.In
}

// Test starts an app and returns fulfilled dependencies
//
// The generic return type T must conform to fx.In such
// that it's dependencies can be fulfilled.
func Test[T any](t testing.TB, opts ...fx.Option) T {
	var deps T
	delayed := newDelayedFxInvocation(func(d T) {
		deps = d
	})

	app := fxtest.New(
		t,
		FxAgentBase(),
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		delayed.option(),
		fx.Options(opts...),
	)
	app.RequireStart()

	t.Cleanup(func() {
		app.RequireStop()
	})

	if err := delayed.call(); err != nil {
		t.Fatal(err.Error())
	}

	return deps
}

// TestApp starts an fx app and returns fulfilled dependencies
//
// The generic return type T must conform to fx.In such
// that it's dependencies can be fulfilled.
func TestApp[T any](opts ...fx.Option) (*fx.App, T, error) {
	var deps T
	delayed := newDelayedFxInvocation(func(d T) {
		deps = d
	})

	app := fx.New(
		FxAgentBase(),
		delayed.option(),
		fx.Options(opts...),
	)
	var err error
	if err = app.Start(context.TODO()); err != nil {
		return nil, deps, err
	}

	err = delayed.call()

	return app, deps, err
}

type appAssertFn func(testing.TB, *fx.App)

// TestStart runs an app fx.App.
//
// This function does *not* leverage fxtest.App because we want to be
// able to test for App initialization errors and expected failures.
//
// The given function is called after the app's startup has completed, with its
// arguments filled via Fx's dependency injection.  The provided testing.TB
// argument will be used for the appAssertFn hook, but the test will not automatically
// fail if the application fails to start.
//
// The supplied `fn` function will never be called, but is required to setup
// that arg appropriately
//
// Use `fx.Options(..)` to bundle multiple fx.Option values into one.
func TestStart(t testing.TB, opts fx.Option, appAssert appAssertFn, fn interface{}) {
	delayed := newDelayedFxInvocation(fn)
	app := fx.New(
		FxAgentBase(),
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		delayed.option(),
		opts,
	)

	appAssert(t, app)
}

// TestRun is a helper for testing code that uses fxutil.Run
//
// It takes a anonymous function, and sets up fx so that no actual App
// will be constructed. Instead, it expects the given function to call
// fxutil.Run. Then, this test verifies that all Options given to that
// fxutil.Run call will satisfy fx's dependences by using fx.ValidateApp.
func TestRun(t *testing.T, f func() error) {
	var fxFakeAppRan bool
	fxAppTestOverride = func(_ interface{}, opts []fx.Option) error {
		fxFakeAppRan = true
		opts = append(opts, FxAgentBase())
		require.NoError(t, fx.ValidateApp(opts...))
		return nil
	}
	defer func() { fxAppTestOverride = nil }()
	require.NoError(t, f())
	require.True(t, fxFakeAppRan, "fxutil.Run wasn't called")
}

// TestRunWithApp is a helper for testing code that uses fxutil.Run
//
// It uses the same logic as fxutil.Run to start a fx App, but returns the App
// after starting in order to test that the App can stop gracefully.
func TestRunWithApp(opts ...fx.Option) (*fx.App, error) {
	if fxAppTestOverride != nil {
		return nil, fxAppTestOverride(func() {}, opts)
	}

	opts = append(opts, FxAgentBase())
	// Temporarily increase timeout for all fxutil.Run calls until we can better characterize our
	// start time requirements. Prepend to opts so individual calls can override the timeout.
	opts = append(
		[]fx.Option{TemporaryAppTimeouts()},
		opts...,
	)
	app := fx.New(opts...)

	if err := app.Err(); err != nil {
		return app, err
	}

	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return app, errors.Join(UnwrapIfErrArgumentsFailed(err), stopApp(app))
	}

	return app, nil
}

// TestOneShotSubcommand is a helper for testing commands implemented with fxutil.OneShot.
//
// It takes an array of commands, and attaches all to a temporary top-level
// command, then executes the given command line (beginning with the
// subcommand name) against that top-level command.
//
// The execution should eventually call fxutil.OneShot with the oneShotFunc
// given by expectedOneShotFunc.  However, this function will not actually be
// called, as that would lead to the one-shot command actually running.  It
// is validated with fx.ValidateApp, however.
//
// The `fx.Option`s passed to fxutil.OneShot are used to create a new app
// containing only the final argument to this function.  Be careful not to
// require any components, since nothing is mocked here.  Typically, the
// function only requires static values such as `BundleParams` or `cliParams`
// and asserts they contain appropriate values.
func TestOneShotSubcommand(
	t *testing.T,
	subcommands []*cobra.Command,
	commandline []string,
	expectedOneShotFunc interface{},
	verifyFn interface{},
) {
	var oneShotRan bool
	fxAppTestOverride = func(oneShotFunc interface{}, opts []fx.Option) error {
		oneShotRan = true

		// verify that the expected oneShotFunc would have been called
		require.Equal(t,
			reflect.ValueOf(expectedOneShotFunc).Pointer(),
			reflect.ValueOf(oneShotFunc).Pointer(),
			"got a different oneShotFunc than expected")

		// validate the app with the original oneShotFunc, to ensure that
		// any types it requires are provided.
		require.NoError(t,
			fx.ValidateApp(
				append(opts,
					FxAgentBase(),
					fx.Invoke(oneShotFunc))...))

		// build an app without the oneShotFunc, and with verifyFn
		app := fxtest.New(t,
			append(opts,
				FxAgentBase(),
				fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
				fx.Invoke(verifyFn))...)
		defer app.RequireStart().RequireStop()
		return nil
	}
	defer func() { fxAppTestOverride = nil }()

	cmd := &cobra.Command{Use: "test"}
	for _, c := range subcommands {
		cmd.AddCommand(c)
	}
	cmd.SetArgs(slices.Clone(commandline))

	require.NoError(t, cmd.Execute())
	require.True(t, oneShotRan, "fxutil.OneShot wasn't called")
}

// TestOneShot is a helper for testing there is no missing dependencies when calling
// fxutil.OneShot.
//
// The function passed as the first argument of fx.OneShot is not called. It
// is validated with fx.ValidateApp, however.
func TestOneShot(t *testing.T, fct func()) {
	var oneShotRan bool
	fxAppTestOverride = func(oneShotFunc interface{}, opts []fx.Option) error {
		oneShotRan = true
		// validate the app with the original oneShotFunc, to ensure that
		// any types it requires are provided.
		require.NoError(t,
			fx.ValidateApp(
				append(opts,
					FxAgentBase(),
					fx.Invoke(oneShotFunc))...))
		return nil
	}
	defer func() { fxAppTestOverride = nil }()

	fct()
	require.True(t, oneShotRan, "fxutil.OneShot wasn't called")
}

// TestBundle is an helper to test Bundle.
//
// This function checks that all components built with fx.Provide inside a bundle can be instanciated.
// To do so, it creates an `fx.Invoke(_ component1, _ component2, ...)` and call fx.ValidateApp
func TestBundle(t *testing.T, bundle BundleOptions, extraOptions ...fx.Option) {
	var componentTypes []reflect.Type

	for _, option := range bundle.Options {
		module, ok := option.(Module)
		if ok {
			t.Logf("Discovering components for %v", module)
			for _, moduleOpt := range module.Options {
				componentTypes = appendModuleComponentTypes(t, componentTypes, moduleOpt)
			}
		}
	}
	invoke := createFxInvokeOption(componentTypes)

	t.Logf("Check the following components are instanciable: %v", componentTypes)
	require.NoError(t, fx.ValidateApp(
		invoke,
		bundle,
		fx.Options(extraOptions...),
		FxAgentBase(),
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
	))
}

// appendModuleComponentTypes appends the components inside provideOption to componentTypes
func appendModuleComponentTypes(t *testing.T, componentTypes []reflect.Type, provideOption fx.Option) []reflect.Type {
	moduleValue := reflect.ValueOf(provideOption)
	// provideOption has a `Targets`` field of factories: https://github.com/uber-go/fx/blob/master/provide.go#L65-L68
	targets := moduleValue.FieldByName("Targets")
	if targets.IsValid() {
		targetValues := targets.Interface().([]interface{})
		for _, target := range targetValues {
			targetType := reflect.TypeOf(target)
			if targetType.Kind() == reflect.Func && targetType.NumOut() > 0 {
				// As the first returned type is the component it is enough to consider
				// only the first type
				returnType := targetType.Out(0)
				types := getComponents(t, returnType)
				componentTypes = append(componentTypes, types...)
			}
		}
	}
	return componentTypes
}

// getComponents returns the component contained in mainType.
func getComponents(t *testing.T, mainType reflect.Type) []reflect.Type {
	if isFxOutType(mainType) {
		var types []reflect.Type
		for i := 0; i < mainType.NumField(); i++ {
			field := mainType.Field(i)
			fieldType := field.Type

			// Ignore fx groups because returning an instance of
			// type Provider struct {
			//   fx.Out
			//   Provider MyProvider `group:"myGroup"`
			// }
			// doesn't satisfy fx.Invoke(_ MyProvider)
			if fieldType != fxOutType && field.Tag.Get("group") == "" {
				types = append(types, getComponents(t, fieldType)...)
			}
		}
		return types
	}

	if mainType.Kind() == reflect.Interface || mainType.Kind() == reflect.Struct {
		t.Logf("\tFound: %v", mainType)
		return []reflect.Type{mainType}
	}
	return nil
}

func isFxOutType(t reflect.Type) bool {
	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			fieldType := t.Field(i).Type
			if fieldType == fxOutType {
				return true
			}
		}
	}
	return false
}

// createFxInvokeOption creates fx.Invoke(_ componentTypes[0], _ componentTypes[1], ...)
func createFxInvokeOption(componentTypes []reflect.Type) fx.Option {
	fctSig := reflect.FuncOf(componentTypes, nil, false)
	captureArgs := reflect.MakeFunc(
		fctSig,
		func(_ []reflect.Value) []reflect.Value {
			return []reflect.Value{}
		})

	return fx.Invoke(captureArgs.Interface())
}
