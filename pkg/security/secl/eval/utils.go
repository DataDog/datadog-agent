// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"github.com/pkg/errors"
)

// NotOfValue returns the NOT of a value
func NotOfValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return ^v, nil
	case string:
		return RandString(256), nil
	case bool:
		return !v, nil
	}

	return nil, errors.New("value type unknown")
}
