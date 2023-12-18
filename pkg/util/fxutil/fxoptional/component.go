// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxoptional

import "github.com/DataDog/datadog-agent/pkg/util/optional"

// Component is an optional component type.
// The main purpose of this component is to make it explicit that the component may not be present.
// Here is an full example of how to use it:
//
//	// ------------------------------------------
//	// Component.go
//	// ------------------------------------------
//	type Component fxoptional.Component[MyInterface]
//
//	type MyInterface interface {
//	}
//
//	// ------------------------------------------
//	// Another file implements the component
//	// ------------------------------------------
//
//	// Module defines the fx options for this component.
//	func Module() fxutil.Module {
//		return fxutil.Component(
//			fx.Provide(newMyComponent))
//	}
//
//	type dependencies struct {
//		fx.In
//
//		Config    config.Component
//		Lifecycle fx.Lifecycle
//	}
//
//	type myComponent struct{}
//
//	func newMyComponent(deps dependencies) Component {
//		if !deps.Config.GetBool("mycomponent.enabled") {
//			return fxoptional.None[MyInterface]()
//		}
//
//		// Use lifecycle hooks if needed by the component
//		deps.Lifecycle.Append(fx.Hook{
//			OnStart: func(context.Context) error {
//				return nil
//			},
//			OnStop: func(context.Context) error {
//				return nil
//			},
//		})
//		return fxoptional.New[MyInterface](myComponent{})
//	}
//
// // ------------------------------------------
// // Usage
// // ------------------------------------------
//
//	func UseMyComponent(comp Component) {
//		myComp, ok := comp.Get()
//		if ok {
//			// Do something with myComp
//		}
//	}
//
// It is recommended to use fxoptional.Component instead of optional.Option[Component].
// Consider the following example of code
//
//	 func run(myComponent Component) {
//	 }
//
//	func MyFunc() {
//		fxutil.OneShot(run, Module())
//	}
//
// This code fails when using optional.Option[Component] because the type of the component is not Component but optional.Option[Component].
// The user must know that Module() provides an way to create an optional.Option[Component] and not a Component.
// This is not the case with fxoptional.Component.
// Note: This issue already happened in the past and was hard to debug.
type Component[T any] interface {
	Get() (T, bool)
}

// New creates a new optional component.
func New[T any](t T) *optional.Option[T] {
	o := optional.NewOption[T](t)
	return &o
}

// None creates a new empty optional component.
func None[T any]() *optional.Option[T] {
	o := optional.NewNoneOption[T]()
	return &o
}
