// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker,windows

package docker

import "fmt"

const (
	basePath = "c:\\programdata\\docker\\containers"
)

func checkReadAccess() error {
	return fmt.Errorf("Docker tailing from file is not supported on Windows")
}
