// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver && antithesis

package start

import "github.com/antithesishq/antithesis-sdk-go/assert"

// emitBootstrap fires the Antithesis bootstrap property.
func emitBootstrap() {
	assert.Reachable("cluster-agent start() entered", nil)
}
