// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"reflect"

	"go.uber.org/fx"
)

var errorInterface = reflect.TypeOf((*error)(nil)).Elem()

// delayedFxInvocation delays execution of a function, while allowing
// Fx to provide its arguments.
type delayedFxInvocation struct {
	fn    interface{}
	ftype reflect.Type
	args  []reflect.Value
}

// newDelayedFxInvocation creates a new delayedFxInvocation wrapping the given
// function.
//
// The given function can have any number of arguments that will be supplied with Fx.
// It must return nothing or an error.
func newDelayedFxInvocation(fn interface{}) *delayedFxInvocation {
	ftype := reflect.TypeOf(fn)
	if ftype == nil || ftype.Kind() != reflect.Func {
		panic("delayedFxInvocation requires a function as its first argument")
	}

	// verify it returns error
	if ftype.NumOut() > 1 || (ftype.NumOut() == 1 && !ftype.Out(0).Implements(errorInterface)) {
		panic("delayedFxInvocation function must return error or nothing")
	}

	return &delayedFxInvocation{fn: fn, ftype: ftype}
}

// option generates the fx.Option value to include in an fx.App that will
// provide the argument values.
func (i *delayedFxInvocation) option() fx.Option {
	// build an function with the same signature as i.fn that will
	// capture the args and do nothing.
	captureArgs := reflect.MakeFunc(
		i.ftype,
		func(args []reflect.Value) []reflect.Value {
			i.args = args
			// return nothing or a single nil value of type error
			if i.ftype.NumOut() == 0 {
				return []reflect.Value{}
			}
			return []reflect.Value{reflect.Zero(errorInterface)}
		})

	// fx.Invoke that function to capture the args at startup
	return fx.Invoke(captureArgs.Interface())
}

// call calls the underlying function.  The fx.App must have already supplied
// the arguments at this time.  If the delayed function has no return value, then
// this will always return nil.
func (i *delayedFxInvocation) call() error {
	if i.args == nil {
		panic("delayedFxInvocation args have not yet been provided")
	}

	// call the original function with the args captured during app startup
	res := reflect.ValueOf(i.fn).Call(i.args)

	// and return an error if the function returned any non-nil value
	if len(res) > 0 && !res[0].IsNil() {
		err := res[0].Interface().(error)
		return err
	}
	return nil
}
