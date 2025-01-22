// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
)

var (
	obfuscatorLock sync.Mutex
)

func (c *Check) LazyInitObfuscator() *obfuscate.Obfuscator {
	// Ensure thread safe initialization
	obfuscatorLock.Lock()
	defer obfuscatorLock.Unlock()

	if c.obfuscator == nil {
		c.obfuscator = obfuscate.NewObfuscator(obfuscate.Config{SQL: c.config.ObfuscatorOptions})
	}

	return c.obfuscator
}
