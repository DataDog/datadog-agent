// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package source provides a source provider for the OpenTelemetry Collector.
package source

import (
	"context"
	"fmt"
)

// Kind of source
type Kind string

const (
	// InvalidKind is an invalid kind. It is the zero value of Kind.
	InvalidKind Kind = ""
	// HostnameKind is a host source.
	HostnameKind Kind = "host"
	// AWSECSFargateKind is a serverless source on AWS ECS Fargate.
	AWSECSFargateKind Kind = "task_arn"
	// AzureContainerAppsKind is a serverless source on Azure Container Apps.
	AzureContainerAppsKind Kind = "azure_container_apps"
)

// Source represents a telemetry source.
type Source struct {
	// Kind of source (serverless v. host).
	Kind Kind
	// Identifier that uniquely determines the source.
	//
	// Deprecated: use SourceIdentifier.Primary instead for any new call
	// site. This field remains for existing callers during migration
	// (tracked in datadog-agent#51116); it will be removed once all
	// callers have moved to SourceIdentifier.
	Identifier       string
	SourceIdentifier SourceIdentifier
}

// Tag associated to a source.
func (s Source) Tag() string {
	identifier := s.Identifier
	if identifier == "" {
		identifier = s.SourceIdentifier.Primary
	}
	return fmt.Sprintf("%s:%s", s.Kind, identifier)
}

// SourceIdentifier holds the identity of a telemetry source, generalizing the
// single-string Source.Identifier to support workloads that need more than
// one identifying attribute.
type SourceIdentifier struct {
	Primary    string
	Dimensions map[string]string
}

// Provider identifies a source.
type Provider interface {
	// Source gets the source from the current context.
	Source(ctx context.Context) (Source, error)
}
