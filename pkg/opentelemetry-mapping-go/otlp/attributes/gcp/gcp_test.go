// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package gcp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/internal/testutils"
)

const (
	testShortHostname = "hostname"
	testHostID        = "hostID"
	testCloudZone     = "zone"
	testHostType      = "machineType"
	testCloudAccount  = "projectID"
	testHostname      = testShortHostname + ".c." + testCloudAccount + ".internal"
	testBadHostname   = "badhostname"
)

var (
	testFullMap = testutils.NewAttributeMap(map[string]string{
		string(conventions.CloudProviderKey):         conventions.CloudProviderGCP.Value.AsString(),
		string(conventions.HostIDKey):                testHostID,
		string(conventions.HostNameKey):              testHostname,
		string(conventions.CloudAvailabilityZoneKey): testCloudZone,
		string(conventions.HostTypeKey):              testHostType,
		string(conventions.CloudAccountIDKey):        testCloudAccount,
	})

	testFullBadMap = testutils.NewAttributeMap(map[string]string{
		string(conventions.CloudProviderKey):         conventions.CloudProviderGCP.Value.AsString(),
		string(conventions.HostIDKey):                testHostID,
		string(conventions.HostNameKey):              testBadHostname,
		string(conventions.CloudAvailabilityZoneKey): testCloudZone,
		string(conventions.HostTypeKey):              testHostType,
		string(conventions.CloudAccountIDKey):        testCloudAccount,
	})

	testGCPIntegrationHostname    = fmt.Sprintf("%s.%s", testShortHostname, testCloudAccount)
	testGCPIntegrationBadHostname = fmt.Sprintf("%s.%s", testBadHostname, testCloudAccount)
)

func TestInfoFromAttrs(t *testing.T) {
	tags := []string{"instance-id:hostID", "zone:zone", "instance-type:machineType", "project:projectID"}
	tests := []struct {
		name  string
		attrs pcommon.Map

		ok          bool
		hostname    string
		hostAliases []string
		gcpTags     []string
	}{
		{
			name:  "no hostname",
			attrs: testutils.NewAttributeMap(map[string]string{}),
		},
		{
			name:     "hostname",
			attrs:    testFullMap,
			ok:       true,
			hostname: testGCPIntegrationHostname,
			gcpTags:  tags,
		},
		{
			name:     "bad hostname",
			attrs:    testFullBadMap,
			ok:       true,
			hostname: testGCPIntegrationBadHostname,
			gcpTags:  tags,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			hostname, ok := HostnameFromAttrs(testInstance.attrs)
			assert.Equal(t, testInstance.ok, ok)
			assert.Equal(t, testInstance.hostname, hostname)

			hostInfo := HostInfoFromAttrs(testInstance.attrs)
			assert.ElementsMatch(t, testInstance.hostAliases, hostInfo.HostAliases)
			assert.ElementsMatch(t, testInstance.gcpTags, hostInfo.GCPTags)
		})
	}
}
