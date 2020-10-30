// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

import (
	"unsafe"
)

// Context describes the context used during a rule evaluation
type Context struct {
	Object unsafe.Pointer
}

// SetObject set the given object to the context
func (c *Context) SetObject(obj unsafe.Pointer) {
	c.Object = obj
}
