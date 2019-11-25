// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package hostname

import "github.com/DataDog/datadog-agent/pkg/util/docker"

func init() {
	RegisterHostnameProvider("docker", docker.HostnameProvider)
}
