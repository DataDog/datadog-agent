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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/azure"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/internal/testutils"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

const (
	testLiteralHost            = "literal-host"
	testHostID                 = "example-host-id"
	testHostName               = "example-host-name"
	testContainerID            = "example-container-id"
	testClusterName            = "clusterName"
	testNodeName               = "nodeName"
	testCustomName             = "example-custom-host-name"
	testCloudAccount           = "projectID"
	testGCPHostname            = testHostName + ".c." + testCloudAccount + ".internal"
	testGCPIntegrationHostname = testHostName + "." + testCloudAccount
)

func TestSourceFromAttrs(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map

		ok  bool
		src source.Source
	}{
		{
			name: "literal 'host' tag",
			attrs: testutils.NewAttributeMap(map[string]string{
				AttributeHost:                         testLiteralHost,
				AttributeDatadogHostname:              testCustomName,
				AttributeK8sNodeName:                  testNodeName,
				string(conventions.K8SClusterNameKey): testClusterName,
				string(conventions.ContainerIDKey):    testContainerID,
				string(conventions.HostIDKey):         testHostID,
				string(conventions.HostNameKey):       testHostName,
			}),
			ok:  true,
			src: source.Source{Kind: source.HostnameKind, Identifier: testLiteralHost},
		},
		{
			name: "custom hostname",
			attrs: testutils.NewAttributeMap(map[string]string{
				AttributeDatadogHostname:              testCustomName,
				AttributeK8sNodeName:                  testNodeName,
				string(conventions.K8SClusterNameKey): testClusterName,
				string(conventions.ContainerIDKey):    testContainerID,
				string(conventions.HostIDKey):         testHostID,
				string(conventions.HostNameKey):       testHostName,
			}),
			ok:  true,
			src: source.Source{Kind: source.HostnameKind, Identifier: testCustomName},
		},
		{
			name: "container ID",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.ContainerIDKey): testContainerID,
			}),
		},
		{
			name: "AWS EC2",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.CloudProviderKey): conventions.CloudProviderAWS.Value.AsString(),
				string(conventions.HostIDKey):        testHostID,
				string(conventions.HostNameKey):      testHostName,
			}),
			ok:  true,
			src: source.Source{Kind: source.HostnameKind, Identifier: testHostID},
		},
		{
			name: "ECS Fargate",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.CloudProviderKey):      conventions.CloudProviderAWS.Value.AsString(),
				string(conventions.CloudPlatformKey):      conventions.CloudPlatformAWSECS.Value.AsString(),
				string(conventions.AWSECSTaskARNKey):      "example-task-ARN",
				string(conventions.AWSECSTaskFamilyKey):   "example-task-family",
				string(conventions.AWSECSTaskRevisionKey): "example-task-revision",
				string(conventions.AWSECSLaunchtypeKey):   conventions.AWSECSLaunchtypeFargate.Value.AsString(),
			}),
			ok:  true,
			src: source.Source{Kind: source.AWSECSFargateKind, Identifier: "example-task-ARN"},
		},
		{
			name: "GCP",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.CloudProviderKey):  conventions.CloudProviderGCP.Value.AsString(),
				string(conventions.HostIDKey):         testHostID,
				string(conventions.HostNameKey):       testGCPHostname,
				string(conventions.CloudAccountIDKey): testCloudAccount,
			}),
			ok:  true,
			src: source.Source{Kind: source.HostnameKind, Identifier: testGCPIntegrationHostname},
		},
		{
			name: "GCP, no account id",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.CloudProviderKey): conventions.CloudProviderGCP.Value.AsString(),
				string(conventions.HostIDKey):        testHostID,
				string(conventions.HostNameKey):      testGCPHostname,
			}),
		},
		{
			name: "azure",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.CloudProviderKey): conventions.CloudProviderAzure.Value.AsString(),
				string(conventions.HostIDKey):        testHostID,
				string(conventions.HostNameKey):      testHostName,
			}),
			ok:  true,
			src: source.Source{Kind: source.HostnameKind, Identifier: testHostID},
		},
		{
			name: "host id v. hostname",
			attrs: testutils.NewAttributeMap(map[string]string{
				string(conventions.HostIDKey):   testHostID,
				string(conventions.HostNameKey): testHostName,
			}),
			ok:  true,
			src: source.Source{Kind: source.HostnameKind, Identifier: testHostID},
		},
		{
			name:  "no hostname",
			attrs: testutils.NewAttributeMap(map[string]string{}),
		},
		{
			name: "localhost",
			attrs: testutils.NewAttributeMap(map[string]string{
				AttributeDatadogHostname: "127.0.0.1",
			}),
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			source, ok := SourceFromAttrs(testInstance.attrs, nil)
			assert.Equal(t, testInstance.ok, ok)
			assert.Equal(t, testInstance.src, source)
		})

	}
}

func TestLiteralHostNonString(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutInt(AttributeHost, 1000)
	src, ok := SourceFromAttrs(attrs, nil)
	assert.True(t, ok)
	assert.Equal(t, source.Source{Kind: source.HostnameKind, Identifier: "1000"}, src)
}

func TestGetClusterName(t *testing.T) {
	// OpenTelemetry convention
	attrs := testutils.NewAttributeMap(map[string]string{
		string(conventions.K8SClusterNameKey): testClusterName,
	})
	cluster, ok := getClusterName(attrs)
	assert.True(t, ok)
	assert.Equal(t, cluster, testClusterName)

	// Azure
	attrs = testutils.NewAttributeMap(map[string]string{
		string(conventions.CloudProviderKey): conventions.CloudProviderAzure.Value.AsString(),
		azure.AttributeResourceGroupName:     "MC_aks-kenafeh_aks-kenafeh-eu_westeurope",
	})
	cluster, ok = getClusterName(attrs)
	assert.True(t, ok)
	assert.Equal(t, cluster, "aks-kenafeh-eu")

	// AWS
	attrs = testutils.NewAttributeMap(map[string]string{
		string(conventions.CloudProviderKey):        conventions.CloudProviderAWS.Value.AsString(),
		"ec2.tag.kubernetes.io/cluster/clustername": "dummy_value",
	})
	cluster, ok = getClusterName(attrs)
	assert.True(t, ok)
	assert.Equal(t, cluster, "clustername")

	// None
	attrs = testutils.NewAttributeMap(map[string]string{})
	_, ok = getClusterName(attrs)
	assert.False(t, ok)
}

func TestHostnameKubernetes(t *testing.T) {
	// Node name and cluster name
	attrs := testutils.NewAttributeMap(map[string]string{
		AttributeK8sNodeName:                  testNodeName,
		string(conventions.K8SClusterNameKey): testClusterName,
		string(conventions.ContainerIDKey):    testContainerID,
		string(conventions.HostIDKey):         testHostID,
		string(conventions.HostNameKey):       testHostName,
	})
	hostname, ok := hostnameFromAttributes(attrs)
	assert.True(t, ok)
	assert.Equal(t, hostname, "nodeName-clusterName")

	// Node name, no cluster name
	attrs = testutils.NewAttributeMap(map[string]string{
		AttributeK8sNodeName:               testNodeName,
		string(conventions.ContainerIDKey): testContainerID,
		string(conventions.HostIDKey):      testHostID,
		string(conventions.HostNameKey):    testHostName,
	})
	hostname, ok = hostnameFromAttributes(attrs)
	assert.True(t, ok)
	assert.Equal(t, hostname, "nodeName")

	// Node name, no cluster name, AWS EC2
	attrs = testutils.NewAttributeMap(map[string]string{
		AttributeK8sNodeName:                 testNodeName,
		string(conventions.ContainerIDKey):   testContainerID,
		string(conventions.HostIDKey):        testHostID,
		string(conventions.HostNameKey):      testHostName,
		string(conventions.CloudProviderKey): conventions.CloudProviderAWS.Value.AsString(),
	})
	hostname, ok = hostnameFromAttributes(attrs)
	assert.True(t, ok)
	assert.Equal(t, hostname, testHostID)

	// no node name, cluster name
	attrs = testutils.NewAttributeMap(map[string]string{
		string(conventions.K8SClusterNameKey): testClusterName,
		string(conventions.ContainerIDKey):    testContainerID,
		string(conventions.HostIDKey):         testHostID,
		string(conventions.HostNameKey):       testHostName,
	})
	hostname, ok = hostnameFromAttributes(attrs)
	assert.True(t, ok)
	// cluster name gets ignored, fallback to next option
	assert.Equal(t, hostname, testHostID)
}
