// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitImageName(t *testing.T) {
	for nb, tc := range []struct {
		source    string
		longName  string
		registry  string
		shortName string
		tag       string
		err       error
	}{
		// Empty
		{"", "", "", "", "", fmt.Errorf("empty image name")},
		// A sha256 string
		{"sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0", "", "", "", "", fmt.Errorf("invalid image name (is a sha256)")},
		// Shortest possibility
		{"alpine", "alpine", "", "alpine", "", nil},
		// Historical docker format
		{"nginx:latest", "nginx", "", "nginx", "latest", nil},
		// Org prefix to be removed for short name
		{"datadog/docker-dd-agent:latest-jmx",
			"datadog/docker-dd-agent", "", "docker-dd-agent", "latest-jmx", nil},
		// Sha-pinning used by many orchestrators -> empty tag
		// We should not have this string here as ResolveImageName should
		// have handled that before, but let's keep it just in case
		{"redis@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
			"redis", "", "redis", "", nil},
		// Quirky pinning used by swarm
		{"org/redis:latest@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
			"org/redis", "", "redis", "latest", nil},
		// Custom registry, simple form
		{"myregistry.local:5000/testing/test-image:version",
			"myregistry.local:5000/testing/test-image", "myregistry.local:5000", "test-image", "version", nil},
		// Custom registry, most insane form possible
		{"myregistry.local:5000/testing/test-image:version@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
			"myregistry.local:5000/testing/test-image", "myregistry.local:5000", "test-image", "version", nil},
		// Test swarm image
		{"dockercloud/haproxy:1.6.7@sha256:8c4ed4049f55de49cbc8d03d057a5a7e8d609c264bb75b59a04470db1d1c5121",
			"dockercloud/haproxy", "", "haproxy", "1.6.7", nil},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.source), func(t *testing.T) {
			assert := assert.New(t)
			long, registry, short, tag, err := SplitImageName(tc.source)
			assert.Equal(tc.longName, long)
			assert.Equal(tc.registry, registry)
			assert.Equal(tc.shortName, short)
			assert.Equal(tc.tag, tag)

			if tc.err == nil {
				assert.Nil(err)
			} else {
				assert.NotNil(err)
				assert.Equal(tc.err.Error(), err.Error())
			}
		})
	}
}
