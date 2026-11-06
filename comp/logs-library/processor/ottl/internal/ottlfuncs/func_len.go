// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Adapted for Agent JSON/body values: Collector log, metric, and trace slices
// cannot be returned by this context and must not become production imports.

package ottlfuncs

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

const (
	typeError = `target arg must be of type string, []any, map[string]any, pcommon.Map, pcommon.Slice, pcommon.Value (of type String, Map, Slice) or a supported slice type from the plog, pmetric or ptrace packages`
)

type lenArguments[K any] struct {
	Target ottl.Getter[K]
}

// NewLenFactory returns a factory for the Len OTTL function.
// See https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/ottl/ottlfuncs/README.md#len
func NewLenFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("Len", &lenArguments[K]{}, createLenFunction[K])
}

func createLenFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*lenArguments[K])

	if !ok {
		return nil, errors.New("LenFactory args must be of type *lenArguments[K]")
	}

	return computeLen(args.Target), nil
}

func computeLen[K any](target ottl.Getter[K]) ottl.ExprFunc[K] {
	return func(ctx context.Context, tCtx K) (any, error) {
		val, err := target.Get(ctx, tCtx)
		if err != nil {
			return nil, err
		}

		switch valType := val.(type) {
		case pcommon.Value:
			switch valType.Type() {
			case pcommon.ValueTypeStr:
				return int64(len(valType.Str())), nil
			case pcommon.ValueTypeSlice:
				return int64(valType.Slice().Len()), nil
			case pcommon.ValueTypeMap:
				return int64(valType.Map().Len()), nil
			}
			return nil, fmt.Errorf("Len: unsupported target type %T. %s", val, typeError)
		case pcommon.Map:
			return int64(valType.Len()), nil
		case pcommon.Slice:
			return int64(valType.Len()), nil
		}

		v := reflect.ValueOf(val)
		switch v.Kind() {
		case reflect.String, reflect.Map, reflect.Slice:
			return int64(v.Len()), nil
		}

		return nil, fmt.Errorf("Len: unsupported target type %T. %s", val, typeError)
	}
}
