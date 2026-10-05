// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package productcomposition

import "go.uber.org/fx"

func autodiscoveryOptions() []fx.Option {
	return nil
}

func hostMetadataOptions() []fx.Option {
	return nil
}

func collectorOptions() []fx.Option {
	return nil
}

func guiOptions() []fx.Option {
	return nil
}

var pythonVersionGetFunc func() string
