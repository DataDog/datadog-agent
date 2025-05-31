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

package ec2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/internal/testutils"
)

const (
	testIP         = "ip-12-34-56-78.us-west-2.compute.internal"
	testDomu       = "domu-12-34-56-78.us-west-2.compute.internal"
	testEC2        = "ec2amaz-12-34-56-78.us-west-2.compute.internal"
	customHost     = "custom-hostname"
	testInstanceID = "i-0123456789"
)

func TestDefaultHostname(t *testing.T) {
	assert.True(t, isDefaultHostname(testIP))
	assert.True(t, isDefaultHostname(testDomu))
	assert.True(t, isDefaultHostname(testEC2))
	assert.False(t, isDefaultHostname(customHost))
}

func TestHostnameFromAttrs(t *testing.T) {
	t.Run("host id", func(t *testing.T) {
		attrs := testutils.NewAttributeMap(map[string]string{
			string(conventions.CloudProviderKey): conventions.CloudProviderAWS.Value.AsString(),
			string(conventions.HostIDKey):        testInstanceID,
			string(conventions.HostNameKey):      testIP,
		})
		hostname, ok := HostnameFromAttrs(attrs)
		assert.True(t, ok)
		assert.Equal(t, hostname, testInstanceID)
	})

	t.Run("no host id", func(t *testing.T) {
		attrs := testutils.NewAttributeMap(map[string]string{
			string(conventions.CloudProviderKey): conventions.CloudProviderAWS.Value.AsString(),
			string(conventions.HostNameKey):      testIP,
		})
		hostname, ok := HostnameFromAttrs(attrs)
		assert.False(t, ok)
		assert.Equal(t, hostname, "")
	})
}

func TestHostInfoFromAttributes(t *testing.T) {
	attrs := testutils.NewAttributeMap(map[string]string{
		string(conventions.CloudProviderKey): conventions.CloudProviderAWS.Value.AsString(),
		string(conventions.HostIDKey):        testInstanceID,
		string(conventions.HostNameKey):      testIP,
		"ec2.tag.tag1":                       "val1",
		"ec2.tag.tag2":                       "val2",
		"ignored":                            "ignored",
	})

	hostInfo := HostInfoFromAttributes(attrs)

	assert.Equal(t, hostInfo.InstanceID, testInstanceID)
	assert.Equal(t, hostInfo.EC2Hostname, testIP)
	assert.ElementsMatch(t, hostInfo.EC2Tags, []string{"tag1:val1", "tag2:val2"})
}

func TestClusterNameFromAttributes(t *testing.T) {
	cluster, ok := ClusterNameFromAttributes(testutils.NewAttributeMap(map[string]string{
		"ec2.tag.kubernetes.io/cluster/clustername": "dummy_value",
	}))
	assert.True(t, ok)
	assert.Equal(t, cluster, "clustername")

	_, ok = ClusterNameFromAttributes(testutils.NewAttributeMap(map[string]string{}))
	assert.False(t, ok)
}
