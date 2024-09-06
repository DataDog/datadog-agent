// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import "github.com/google/uuid"

// NewPathtraceID creates a new pathtrace id
func NewPathtraceID() string {
	return uuid.New().String()
}
