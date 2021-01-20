// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker,windows

package docker

import (
	"io/ioutil"
)

const (
	basePath = "c:\\programdata\\docker\\containers"
)

func checkReadAccess() error {
	// We need read access to the docker folder
	_, err := ioutil.ReadDir(basePath)
	return err
}
