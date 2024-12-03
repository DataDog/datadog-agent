// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package eval holds eval related files
package eval

// AppendResolvedField instructs the context that this field has been resolved
func (c *Context) AppendResolvedField(field string) {
	if field == "" {
		return
	}

	c.resolvedFields = append(c.resolvedFields, field)
}
