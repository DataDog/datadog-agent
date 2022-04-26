// Copyright  OpenTelemetry Authors
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
	conventions "go.opentelemetry.io/collector/model/semconv/v1.6.1"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/attributes/azure"
	"github.com/DataDog/datadog-agent/pkg/otlp/model/attributes/ec2"
	"github.com/DataDog/datadog-agent/pkg/otlp/model/attributes/gcp"
)

const (
	// AttributeDatadogHostname the datadog host name attribute
	AttributeDatadogHostname = "datadog.host.name"
	// AttributeK8sNodeName the datadog k8s node name attribute
	AttributeK8sNodeName = "k8s.node.name"
)

func getClusterName(attrs pcommon.Map) (string, bool) {
	if k8sClusterName, ok := attrs.Get(conventions.AttributeK8SClusterName); ok {
		return k8sClusterName.StringVal(), true
	}

	cloudProvider, ok := attrs.Get(conventions.AttributeCloudProvider)
	if ok && cloudProvider.StringVal() == conventions.AttributeCloudProviderAzure {
		return azure.ClusterNameFromAttributes(attrs)
	} else if ok && cloudProvider.StringVal() == conventions.AttributeCloudProviderAWS {
		return ec2.ClusterNameFromAttributes(attrs)
	}

	return "", false
}

// HostnameFromAttributes tries to get a valid hostname from attributes by checking, in order:
//
//   1. a custom Datadog hostname provided by the "datadog.host.name" attribute
//   2. the Kubernetes node name (and cluster name if available),
//   3. cloud provider specific hostname for AWS or GCP
//   4. the container ID,
//   5. the cloud provider host ID and
//   6. the host.name attribute.
//
//  It returns a boolean value indicated if any name was found
func HostnameFromAttributes(attrs pcommon.Map) (string, bool) {
	// Check if the host is localhost or 0.0.0.0, if so discard it.
	// We don't do the more strict validation done for metadata,
	// to avoid breaking users existing invalid-but-accepted hostnames.
	var invalidHosts = map[string]struct{}{
		"0.0.0.0":                 {},
		"127.0.0.1":               {},
		"localhost":               {},
		"localhost.localdomain":   {},
		"localhost6.localdomain6": {},
		"ip6-localhost":           {},
	}

	candidateHost, ok := unsanitizedHostnameFromAttributes(attrs)
	if _, invalid := invalidHosts[candidateHost]; invalid {
		return "", false
	}
	return candidateHost, ok
}

func unsanitizedHostnameFromAttributes(attrs pcommon.Map) (string, bool) {
	// Custom hostname: useful for overriding in k8s/cloud envs
	if customHostname, ok := attrs.Get(AttributeDatadogHostname); ok {
		return customHostname.StringVal(), true
	}

	if launchType, ok := attrs.Get(conventions.AttributeAWSECSLaunchtype); ok && launchType.StringVal() == conventions.AttributeAWSECSLaunchtypeFargate {
		// If on AWS ECS Fargate, return a valid but empty hostname
		return "", true
	}

	// Kubernetes: node-cluster if cluster name is available, else node
	if k8sNodeName, ok := attrs.Get(AttributeK8sNodeName); ok {
		if k8sClusterName, ok := getClusterName(attrs); ok {
			return k8sNodeName.StringVal() + "-" + k8sClusterName, true
		}
		return k8sNodeName.StringVal(), true
	}

	cloudProvider, ok := attrs.Get(conventions.AttributeCloudProvider)
	if ok && cloudProvider.StringVal() == conventions.AttributeCloudProviderAWS {
		return ec2.HostnameFromAttributes(attrs)
	} else if ok && cloudProvider.StringVal() == conventions.AttributeCloudProviderGCP {
		return gcp.HostnameFromAttributes(attrs)
	} else if ok && cloudProvider.StringVal() == conventions.AttributeCloudProviderAzure {
		return azure.HostnameFromAttributes(attrs)
	}

	// host id from cloud provider
	if hostID, ok := attrs.Get(conventions.AttributeHostID); ok {
		return hostID.StringVal(), true
	}

	// hostname from cloud provider or OS
	if hostName, ok := attrs.Get(conventions.AttributeHostName); ok {
		return hostName.StringVal(), true
	}

	// container id (e.g. from Docker)
	if containerID, ok := attrs.Get(conventions.AttributeContainerID); ok {
		return containerID.StringVal(), true
	}

	return "", false
}
