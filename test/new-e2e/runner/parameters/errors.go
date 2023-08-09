// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import "fmt"

// ParameterNotFoundError exported type should have comment or be unexported
type ParameterNotFoundError struct {
	key StoreKey
}

func (e ParameterNotFoundError) Error() string {
	return fmt.Sprintf("parameter %v not found", e.key)
}
