// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
)

// wrapErrorWithID wraps an error with an ID (e.g. rule ID)
func wrapErrorWithID(id string, err error) error {
	return fmt.Errorf("%s: %w", id, err)
}
