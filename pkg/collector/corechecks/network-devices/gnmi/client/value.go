// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"encoding/json"
	"errors"
	"fmt"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

func decodeTypedValue(value *gnmipb.TypedValue) (any, error) {
	if value == nil {
		return nil, errors.New("typed value is nil")
	}

	switch typed := value.GetValue().(type) {
	case *gnmipb.TypedValue_StringVal:
		return typed.StringVal, nil
	case *gnmipb.TypedValue_IntVal:
		return typed.IntVal, nil
	case *gnmipb.TypedValue_UintVal:
		return typed.UintVal, nil
	case *gnmipb.TypedValue_BoolVal:
		return typed.BoolVal, nil
	case *gnmipb.TypedValue_DoubleVal:
		return typed.DoubleVal, nil
	case *gnmipb.TypedValue_LeaflistVal:
		return typed.LeaflistVal.GetElement(), nil
	case *gnmipb.TypedValue_JsonVal:
		return decodeJSONValue(typed.JsonVal)
	case *gnmipb.TypedValue_JsonIetfVal:
		return decodeJSONValue(typed.JsonIetfVal)
	case *gnmipb.TypedValue_AsciiVal:
		return typed.AsciiVal, nil
	case *gnmipb.TypedValue_ProtoBytes:
		return typed.ProtoBytes, nil
	case *gnmipb.TypedValue_BytesVal:
		return typed.BytesVal, nil
	default:
		return nil, fmt.Errorf("unsupported typed value %T", typed)
	}
}

func decodeJSONValue(raw []byte) (any, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw), nil
	}
	return decoded, nil
}
