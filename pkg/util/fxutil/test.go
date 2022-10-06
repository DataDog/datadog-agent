// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// Test runs a test case within an fx.App.
//
// The given function is called after the app's startup has completed, with its
// arguments filled via Fx's dependency injection.  Within the app, `t` is
// provided as type `testing.TB`.
//
// Use `fx.Options(..)` to bundle multiple fx.Option values into one.
func Test(t testing.TB, opts fx.Option, fn interface{}) {
	delayed := newDelayedFxInvocation(fn)
	app := fxtest.New(
		t,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		delayed.option(),
		opts,
	)
	defer app.RequireStart().RequireStop()

	if err := delayed.call(); err != nil {
		t.Fatal(err.Error())
	}
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
	verifyFn interface{}) {

	var oneShotRan bool
	oneShotTestOverride = func(oneShotFunc interface{}, opts []fx.Option) error {
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
					fx.Invoke(oneShotFunc))...))

		// build an app without the oneShotFunc, and with verifyFn
		app := fxtest.New(t,
			append(opts,
				fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
				fx.Invoke(verifyFn))...)
		defer app.RequireStart().RequireStop()
		return nil
	}
	defer func() { oneShotTestOverride = nil }()

	cmd := &cobra.Command{Use: "test"}
	for _, c := range subcommands {
		cmd.AddCommand(c)
	}
	cmd.SetArgs(append([]string{}, commandline...))

	require.NoError(t, cmd.Execute())
	require.True(t, oneShotRan, "fxutil.OneShot wasn't called")
}
