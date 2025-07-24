// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import "fmt"

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

	// Init bootstraps the CloudService.
	Init() error

	// GetStartMetricName returns the metric name for start events
	GetStartMetricName() string
}

//nolint:revive // TODO(SERV) Fix revive linter
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
	return "datadog.serverless_agent"
}

// Init is not necessary for LocalService
func (l *LocalService) Init() error {
	return nil
}

// GetStartMetricName returns the metric name for container start (coldstart) events
func (l *LocalService) GetStartMetricName() string {
	return fmt.Sprintf("%s.enhanced.cold_start", l.GetPrefix())
}

// GetCloudServiceType TODO: Refactor to avoid leaking individual service implementation details into the interface layer
//
//nolint:revive // TODO(SERV) Fix revive lin
//nolint:revive // TODO(SERV) Fix revive linter
func GetCloudServiceType() CloudService {
	if isCloudRunService() {
		if isCloudRunFunction() {
			return &CloudRun{spanNamespace: cloudRunFunction}
		}
		return &CloudRun{spanNamespace: cloudRunService}
	}

	if isCloudRunJob() {
		return &CloudRunJobs{}
	}

	if isContainerAppService() {
		return NewContainerApp()
	}

	if isAppService() {
		return &AppService{}
	}

	return &LocalService{}
}
