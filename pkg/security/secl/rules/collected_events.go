// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

// CollectedEvent exported type should have comment or be unexported
type CollectedEvent struct {
	Type       string
	EvalResult bool
	Fields     map[string]interface{}
}
