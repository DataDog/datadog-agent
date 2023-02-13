// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

// CloudService implements getting tags from each Cloud Provider.
type CloudService interface {
	// GetTags returns a map of tags for a given cloud service. These tags are then attached to
	// the logs, traces, and metrics.
	GetTags() map[string]string

	// GetOrigin returns the value that will be used for the `origin` attribute for
	// all logs, traces, and metrics.
	GetOrigin() string

	// GetPrefix returns the prefix that we're prefixing all
	// metrics with. For example, for cloudrun, we're using
	// gcp.run.{metric_name}. In this example, `gcp.run` is the
	// prefix.
	GetPrefix() string
}

type LocalService struct{}

// GetTags is a default implementation that returns a local empty tag set
func (l *LocalService) GetTags() map[string]string {
	return map[string]string{}
}

// GetOrigin is a default implementation that returns a local empty origin
func (l *LocalService) GetOrigin() string {
	return "local"
}

// GetPrefix is a default implementation that returns a local prefix
func (l *LocalService) GetPrefix() string {
	return "local"
}

func GetCloudServiceType() CloudService {
	if isCloudRunService() {
		return &CloudRun{}
	}

	if isContainerAppService() {
		return &ContainerApp{}
	}

	return &LocalService{}
}
