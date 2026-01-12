// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.uber.org/fx"
)

// ProvideOptional takes a type parameter and fx.Provide's a NewOption wrapper for that type
func ProvideOptional[T any]() fx.Option {
	return fx.Provide(func(actualType T) option.Option[T] {
		return option.New[T](actualType)
	})
}
