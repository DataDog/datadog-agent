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
	"go.opentelemetry.io/collector/pdata/pcommon"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/azure"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/ec2"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/gcp"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

const (
	// AttributeDatadogHostname the datadog host name attribute
	AttributeDatadogHostname = "datadog.host.name"
	// AttributeK8sNodeName the datadog k8s node name attribute
	AttributeK8sNodeName = "k8s.node.name"
	// AttributeHost is a literal host tag.
	// We check for this to avoid double tagging.
	AttributeHost = "host"
)

func getClusterName(attrs pcommon.Map) (string, bool) {
	if k8sClusterName, ok := attrs.Get(string(conventions.K8SClusterNameKey)); ok {
		return k8sClusterName.Str(), true
	}

	cloudProvider, ok := attrs.Get(string(conventions.CloudProviderKey))
	if ok && cloudProvider.Str() == conventions.CloudProviderAzure.Value.AsString() {
		return azure.ClusterNameFromAttributes(attrs)
	} else if ok && cloudProvider.Str() == conventions.CloudProviderAWS.Value.AsString() {
		return ec2.ClusterNameFromAttributes(attrs)
	}

	return "", false
}

// hostnameFromAttributes tries to get a valid hostname from attributes by checking, in order:
//
//  1. the "host" attribute to avoid double tagging if present.
//
//  2. a custom Datadog hostname provided by the "datadog.host.name" attribute
//
//  3. cloud provider specific hostname for AWS, Azure or GCP,
//
//  4. the Kubernetes node name (and cluster name if available),
//
//  5. the cloud provider host ID and
//
//  6. the host.name attribute.
//
//     It returns a boolean value indicated if any name was found
func hostnameFromAttributes(attrs pcommon.Map) (string, bool) {
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

func k8sHostnameFromAttributes(attrs pcommon.Map) (string, bool) {
	node, ok := attrs.Get(AttributeK8sNodeName)
	if !ok {
		return "", false
	}

	if cluster, ok := getClusterName(attrs); ok {
		return node.Str() + "-" + cluster, true
	}
	return node.Str(), true
}

func unsanitizedHostnameFromAttributes(attrs pcommon.Map) (string, bool) {
	// Literal 'host' tag. Check and use to avoid double tagging.
	if literalHost, ok := attrs.Get(AttributeHost); ok {
		// Use even if not a string, so that we avoid double tagging if
		// `resource_attributes_as_tags` is true and 'host' has a non-string value.
		return literalHost.AsString(), true
	}

	// Custom hostname: useful for overriding in k8s/cloud envs
	if customHostname, ok := attrs.Get(AttributeDatadogHostname); ok {
		return customHostname.Str(), true
	}

	if launchType, ok := attrs.Get(string(conventions.AWSECSLaunchtypeKey)); ok && launchType.Str() == conventions.AWSECSLaunchtypeFargate.Value.AsString() {
		// If on AWS ECS Fargate, we don't have a hostname
		return "", false
	}

	cloudProvider, ok := attrs.Get(string(conventions.CloudProviderKey))
	switch {
	case ok && cloudProvider.Str() == conventions.CloudProviderAWS.Value.AsString():
		return ec2.HostnameFromAttrs(attrs)
	case ok && cloudProvider.Str() == conventions.CloudProviderGCP.Value.AsString():
		return gcp.HostnameFromAttrs(attrs)
	case ok && cloudProvider.Str() == conventions.CloudProviderAzure.Value.AsString():
		return azure.HostnameFromAttrs(attrs)
	}

	// Kubernetes: node-cluster if cluster name is available, else node
	k8sName, k8sOk := k8sHostnameFromAttributes(attrs)
	if k8sOk {
		return k8sName, true
	}

	// host id from cloud provider
	if hostID, ok := attrs.Get(string(conventions.HostIDKey)); ok {
		return hostID.Str(), true
	}

	// hostname from cloud provider or OS
	if hostName, ok := attrs.Get(string(conventions.HostNameKey)); ok {
		return hostName.Str(), true
	}

	return "", false
}

// HostFromAttributesHandler calls OnHost when a hostname is extracted from attributes.
type HostFromAttributesHandler interface {
	OnHost(string)
}

// SourceFromAttrs gets a telemetry signal source from its attributes.
// Deprecated: Use Translator.ResourceToSource or Translator.AttributesToSource instead.
func SourceFromAttrs(attrs pcommon.Map, hostFromAttributesHandler HostFromAttributesHandler) (source.Source, bool) {
	if launchType, ok := attrs.Get(string(conventions.AWSECSLaunchtypeKey)); ok && launchType.Str() == conventions.AWSECSLaunchtypeFargate.Value.AsString() {
		if taskARN, ok := attrs.Get(string(conventions.AWSECSTaskARNKey)); ok {
			return source.Source{Kind: source.AWSECSFargateKind, Identifier: taskARN.Str()}, true
		}
	}

	if host, ok := hostnameFromAttributes(attrs); ok {
		if hostFromAttributesHandler != nil {
			hostFromAttributesHandler.OnHost(host)
		}
		return source.Source{Kind: source.HostnameKind, Identifier: host}, true
	}

	return source.Source{}, false
}
