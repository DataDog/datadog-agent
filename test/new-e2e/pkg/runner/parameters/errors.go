// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import "fmt"

// ParameterNotFoundError instance is an error related to a key not found in a valu store
type ParameterNotFoundError struct {
	key StoreKey
}

// Error returns a printable ParameterNotFoundError
func (e ParameterNotFoundError) Error() string {
	return fmt.Sprintf("parameter %v not found", e.key)
}
