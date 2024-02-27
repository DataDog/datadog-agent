// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice/helper"
)

const (
	revisionNameEnvVar = "K_REVISION"
	//nolint:revive // TODO(SERV) Fix revive linter
	ServiceNameEnvVar = "K_SERVICE"
)

var metadataHelperFunc = helper.GetMetaData

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct{}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := metadataHelperFunc(helper.GetDefaultConfig()).TagMap()

	revisionName := os.Getenv(revisionNameEnvVar)
	serviceName := os.Getenv(ServiceNameEnvVar)

	if revisionName != "" {
		tags["revision_name"] = revisionName
	}

	if serviceName != "" {
		tags["service_name"] = serviceName
	}

	tags["origin"] = c.GetOrigin()
	tags["_dd.origin"] = c.GetOrigin()

	return tags
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *CloudRun) GetPrefix() string {
	return "gcp.run"
}

// Init is empty for CloudRun
func (c *CloudRun) Init() error {
	return nil
}

func isCloudRunService() bool {
	_, exists := os.LookupEnv(ServiceNameEnvVar)
	return exists
}
