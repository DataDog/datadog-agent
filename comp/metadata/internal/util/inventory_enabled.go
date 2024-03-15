// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InventoryEnabled returs true if 'enable_metadata_collection' and 'inventories_enabled' are set to true in the
// configuration.
func InventoryEnabled(conf config.Reader) bool {
	if !conf.GetBool("enable_metadata_collection") {
		log.Debug("Metadata collection disabled: inventories payload will not be collected nor sent")
		return false
	}
	if !conf.GetBool("inventories_enabled") {
		log.Debug("inventories metadata is disabled: inventories payload will not be collected nor sent")
		return false
	}
	return true
}
