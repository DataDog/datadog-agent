// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
)

// getDockerUtil gets a DockerUtil instance, either returning a memoized value
// or trying to create a new one.
func (tf *factory) getDockerUtil() (*dockerutilPkg.DockerUtil, error) {
	if tf.dockerutil == nil {
		var err error
		tf.dockerutil, err = dockerutilPkg.GetDockerUtil()
		if err != nil {
			return nil, err
		}
	}
	return tf.dockerutil, nil
}
