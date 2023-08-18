// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package middleware

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// Inner returns the wrapped check instance.
func (c *CheckWrapper) Inner() check.Check {
	return c.inner
}

// Wait blocks until Run() finishes execution in another
// goroutine. Does not block if Run() is not executing.
func (c *CheckWrapper) Wait() {
	c.runM.Lock()
	defer c.runM.Unlock()
}
