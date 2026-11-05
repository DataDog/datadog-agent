// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Adapted for Agent JSON/body values: Collector log, metric, and trace slices
// cannot be returned by this context and must not become production imports.
package ottlfuncs

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

type isListArguments[K any] struct {
	Target ottl.Getter[K]
}

// NewIsListFactory returns a factory for the IsList OTTL function.
// See https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/ottl/ottlfuncs/README.md#islist
func NewIsListFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("IsList", &isListArguments[K]{}, createIsListFunction[K])
}

func createIsListFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*isListArguments[K])

	if !ok {
		return nil, errors.New("IsListFactory args must be of type *isListArguments[K]")
	}

	return isList(args.Target), nil
}

func isList[K any](target ottl.Getter[K]) ottl.ExprFunc[K] {
	return func(ctx context.Context, tCtx K) (any, error) {
		val, err := target.Get(ctx, tCtx)
		if err != nil {
			return false, err
		}

		switch valType := val.(type) {
		case pcommon.Value:
			return valType.Type() == pcommon.ValueTypeSlice, nil

		case pcommon.Slice, []string, []bool, []int64, []float64, [][]byte, []any:
			return true, nil
		}

		return false, nil
	}
}
