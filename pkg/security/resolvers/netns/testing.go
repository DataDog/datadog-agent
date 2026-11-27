// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && (functionaltests || test)

package netns

// FlushNamespaces runs the periodic flush of the network namespaces now. With TCX links, the host snapshot it requests
// runs asynchronously, on the TC classifier loop.
func (nr *Resolver) FlushNamespaces() {
	nr.manualFlushNamespaces()
}
