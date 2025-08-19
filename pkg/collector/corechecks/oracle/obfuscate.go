// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"sync"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	obfuscatorLock sync.Mutex
)

// LazyInitObfuscator inits an obfuscator a single time the first time it is called
func (c *Check) LazyInitObfuscator() *obfuscate.Obfuscator {
	// Ensure thread safe initialization
	obfuscatorLock.Lock()
	defer obfuscatorLock.Unlock()

	if c.obfuscator == nil {
		var obfuscaterConfig obfuscate.Config
		if err := structure.UnmarshalKey(pkgconfigsetup.Datadog(), "apm_config.obfuscation", &obfuscaterConfig); err != nil {
			log.Errorf("Failed to unmarshal apm_config.obfuscation: %s", err.Error())
			obfuscaterConfig = obfuscate.Config{}
		}
		obfuscaterConfig.SQL = c.config.ObfuscatorOptions

		c.obfuscator = obfuscate.NewObfuscator(obfuscaterConfig)
	}

	return c.obfuscator
}
