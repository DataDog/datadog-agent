// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// HostMetadataEnabled keeps host reporting out of the AAS extension without
// changing the full Agent's providers or overriding global metadata settings.
func HostMetadataEnabled(conf model.Reader) bool {
	return flavor.GetFlavor() != flavor.Dogstatsd ||
		!conf.GetBool("serverless.aas_extension_metadata_enabled")
}
