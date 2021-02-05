// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func notOfValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return ^v, nil
	case string:
		return utils.RandString(256), nil
	case bool:
		return !v, nil
	}

	return nil, errors.New("value type unknown")
}
