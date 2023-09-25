// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

// ReloaderInterface aims to handle policies reloading triggers
type ReloaderInterface interface {
	Start() error
	Stop()
	Chan() <-chan struct{}
}
