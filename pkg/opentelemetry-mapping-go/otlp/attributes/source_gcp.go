// Copyright OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package attributes

import (
	"regexp"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv143 "go.opentelemetry.io/otel/semconv/v1.43.0"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/gcp"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

// IsGCPServerless reports whether the resource declares a Cloud Run or Cloud
// Functions platform, including incomplete identities and out-of-scope jobs.
// Such resources must not fall back to the Collector's host identity.
func IsGCPServerless(attrs pcommon.Map) bool {
	platform, _ := attrs.Get(string(conventions.CloudPlatformKey))
	return platform.Str() == conventions.CloudPlatformGCPCloudFunctions.Value.AsString() ||
		platform.Str() == conventions.CloudPlatformGCPCloudRun.Value.AsString()
}

type gcpResourceIdentity struct {
	projectID    string
	location     string
	workloadName string
}

var (
	gcpCloudRunServiceResourcePattern  = regexp.MustCompile(`^(?://run\.googleapis\.com/)?projects/([^/]+)/locations/([^/]+)/services/([^/]+)(?:/revisions/[^/]+)?$`)
	gcpCloudRunRevisionResourcePattern = regexp.MustCompile(`^(?://run\.googleapis\.com/)?projects/([^/]+)/locations/([^/]+)/revisions/[^/]+$`)
	gcpCloudFunctionsResourcePattern   = regexp.MustCompile(`^(?://cloudfunctions\.googleapis\.com/)?projects/([^/]+)/locations/([^/]+)/(?:cloudfunctions|functions)/([^/]+)$`)
	gcpCloudRunJobResourcePattern      = regexp.MustCompile(`^(?://run\.googleapis\.com/)?projects/([^/]+)/locations/([^/]+)/jobs/([^/]+)(?:/executions/[^/]+(?:/tasks/[^/]+)?)?$`)
)

func gcpResourceIdentityFromPattern(resourceID string, pattern *regexp.Regexp, includeName bool) gcpResourceIdentity {
	matches := pattern.FindStringSubmatch(resourceID)
	if matches == nil {
		return gcpResourceIdentity{}
	}
	identity := gcpResourceIdentity{
		projectID: matches[1],
		location:  matches[2],
	}
	if includeName {
		identity.workloadName = matches[3]
	}
	return identity
}

func mergeGCPResourceIdentity(parsed gcpResourceIdentity, projectID, location, workloadName string) gcpResourceIdentity {
	if projectID != "" {
		parsed.projectID = projectID
	}
	if location != "" {
		parsed.location = location
	}
	if workloadName != "" {
		parsed.workloadName = workloadName
	}
	return parsed
}

func gcpCloudRunResourceIdentity(attrs pcommon.Map, kind source.Kind) gcpResourceIdentity {
	resourceID := stringAttribute(attrs, string(semconv143.CloudResourceIDKey))
	var parsed gcpResourceIdentity
	switch kind {
	case source.GCPCloudRunKind:
		parsed = gcpResourceIdentityFromPattern(resourceID, gcpCloudRunServiceResourcePattern, true)
		if parsed.projectID == "" {
			parsed = gcpResourceIdentityFromPattern(resourceID, gcpCloudRunRevisionResourcePattern, false)
		}
	case source.GCPCloudFunctionsKind:
		parsed = gcpResourceIdentityFromPattern(resourceID, gcpCloudFunctionsResourcePattern, true)
		if parsed.projectID == "" {
			parsed = gcpResourceIdentityFromPattern(resourceID, gcpCloudRunServiceResourcePattern, true)
		}
	}

	return mergeGCPResourceIdentity(
		parsed,
		stringAttribute(attrs, string(conventions.CloudAccountIDKey)),
		stringAttribute(attrs, string(conventions.CloudRegionKey)),
		stringAttribute(attrs, string(conventions.FaaSNameKey)),
	)
}

func hasGCPCloudRunJobEvidence(attrs pcommon.Map) bool {
	for _, key := range []string{"gcp.cloud_run.job.execution", "gcp.cloud_run.job.task_index"} {
		if _, ok := attrs.Get(key); ok {
			return true
		}
	}
	resourceID := stringAttribute(attrs, string(semconv143.CloudResourceIDKey))
	return gcpCloudRunJobResourcePattern.MatchString(resourceID)
}

func gcpCloudRunJobsSourceFromAttributes(attrs pcommon.Map) (source.Source, bool) {
	platform := stringAttribute(attrs, string(conventions.CloudPlatformKey))
	if platform != conventions.CloudPlatformGCPCloudRun.Value.AsString() || !hasGCPCloudRunJobEvidence(attrs) {
		return source.Source{}, false
	}

	resourceID := stringAttribute(attrs, string(semconv143.CloudResourceIDKey))
	identity := mergeGCPResourceIdentity(
		gcpResourceIdentityFromPattern(resourceID, gcpCloudRunJobResourcePattern, true),
		stringAttribute(attrs, string(conventions.CloudAccountIDKey)),
		stringAttribute(attrs, string(conventions.CloudRegionKey)),
		stringAttribute(attrs, string(conventions.FaaSNameKey)),
	)
	instanceID := stringAttribute(attrs, string(conventions.FaaSInstanceKey))
	if identity.projectID == "" || identity.location == "" || identity.workloadName == "" || instanceID == "" {
		return source.Source{}, false
	}

	return source.Source{
		Kind: source.GCPCloudRunJobsKind,
		SourceIdentifier: source.SourceIdentifier{
			Primary: instanceID,
			Dimensions: map[string]string{
				"project_id": identity.projectID,
				"location":   identity.location,
				"job_name":   identity.workloadName,
				"instance":   instanceID,
			},
		},
	}, true
}

func gcpServerlessSourceFromAttributes(attrs pcommon.Map) (source.Source, bool) {
	platform, _ := attrs.Get(string(conventions.CloudPlatformKey))
	var kind source.Kind
	switch platform.Str() {
	case conventions.CloudPlatformGCPCloudFunctions.Value.AsString():
		// Functions take precedence over their underlying Cloud Run service.
		kind = source.GCPCloudFunctionsKind
	case conventions.CloudPlatformGCPCloudRun.Value.AsString():
		kind = source.GCPCloudRunKind
	default:
		return source.Source{}, false
	}

	if hasGCPCloudRunJobEvidence(attrs) {
		return source.Source{}, false
	}

	identity := gcpCloudRunResourceIdentity(attrs, kind)
	instanceID := stringAttribute(attrs, string(conventions.FaaSInstanceKey))
	if identity.projectID == "" || identity.location == "" || identity.workloadName == "" || instanceID == "" {
		return source.Source{}, false
	}
	dims := map[string]string{
		"project_id":   identity.projectID,
		"location":     identity.location,
		"service_name": identity.workloadName,
		"instance":     instanceID,
	}
	if revision, ok := attrs.Get(string(conventions.FaaSVersionKey)); ok && revision.Str() != "" {
		dims["revision_name"] = revision.Str()
	}
	return source.Source{
		Kind:       kind,
		Identifier: instanceID, //nolint:staticcheck // Populate the legacy field during the SourceIdentifier migration.
		SourceIdentifier: source.SourceIdentifier{
			Primary:    instanceID,
			Dimensions: dims,
		},
	}, true
}

func gkeAutopilotSourceFromAttributes(attrs pcommon.Map) (source.Source, bool) {
	if stringAttribute(attrs, string(conventions.CloudProviderKey)) != conventions.CloudProviderGCP.Value.AsString() ||
		stringAttribute(attrs, string(conventions.CloudPlatformKey)) != "gcp_kubernetes_engine" {
		return source.Source{}, false
	}

	projectID := stringAttribute(attrs, string(conventions.CloudAccountIDKey))
	clusterName := stringAttribute(attrs, string(conventions.K8SClusterNameKey))
	hostname := stringAttribute(attrs, AttributeHost)
	if hostname == "" {
		hostname = stringAttribute(attrs, AttributeDatadogHostname)
	}
	if hostname == "" {
		hostname, _ = gcp.HostnameFromAttrs(attrs)
	}
	if projectID == "" || clusterName == "" || !strings.HasPrefix(strings.ToLower(hostname), "gk3") {
		return source.Source{}, false
	}

	primary := strings.Join([]string{projectID, clusterName, hostname}, ":")
	return source.Source{
		Kind: source.GKEAutopilotKind,
		SourceIdentifier: source.SourceIdentifier{
			Primary: primary,
			Dimensions: map[string]string{
				"project_id":   projectID,
				"cluster_name": clusterName,
				"hostname":     hostname,
			},
		},
	}, true
}
