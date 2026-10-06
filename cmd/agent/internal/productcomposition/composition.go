// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package productcomposition defines product-level Fx composition for the core Agent binary.
package productcomposition

import "github.com/DataDog/datadog-agent/cmd/agent/command"

// CoreAgent returns the product composition for the core Agent binary.
func CoreAgent() command.ProductComposition {
	return command.ProductComposition{
		AutodiscoveryOptions: autodiscoveryOptions(),
		HostMetadataOptions:  hostMetadataOptions(),
		CollectorOptions:     collectorOptions(),
		GUIOptions:           guiOptions(),
		CheckOptions:         checkOptions(),
		PythonVersionGetFunc: pythonVersionGetFunc,
	}
}
