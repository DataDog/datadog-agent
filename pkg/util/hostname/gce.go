// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build gce

package hostname

import "github.com/DataDog/datadog-agent/pkg/util/gce"

func init() {
	RegisterHostnameProvider("gce", gce.HostnameProvider)
}
