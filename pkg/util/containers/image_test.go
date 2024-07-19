// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitImageName(t *testing.T) {
	for nb, tc := range []struct {
		source   string
		expected StructImageName
		err      error
	}{
		// Empty
		{"", StructImageName{}, fmt.Errorf("empty image name")},
		// A sha256 string
		{"sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0", StructImageName{}, fmt.Errorf("invalid image name (is a sha256)")},
		// Shortest possibility
		{"alpine", StructImageName{Long: "alpine", Short: "alpine"}, nil},
		// Historical docker format
		{"nginx:latest", StructImageName{Long: "nginx", Short: "nginx", Tag: "latest"}, nil},
		// Org prefix to be removed for short name
		{"datadog/docker-dd-agent:latest-jmx",
			StructImageName{Long: "datadog/docker-dd-agent", Short: "docker-dd-agent", Tag: "latest-jmx"}, nil},
		// Sha-pinning used by many orchestrators -> empty tag
		// We should not have this string here as ResolveImageName should
		// have handled that before, but let's keep it just in case
		{"redis@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
			StructImageName{Long: "redis", Short: "redis", Digest: "sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0"}, nil},
		// Quirky pinning used by swarm
		{"org/redis:latest@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
			StructImageName{Long: "org/redis", Short: "redis", Tag: "latest", Digest: "sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0"}, nil},
		// Custom registry, simple form
		{"myregistry.local:5000/testing/test-image:version",
			StructImageName{Long: "myregistry.local:5000/testing/test-image", Registry: "myregistry.local:5000", Short: "test-image", Tag: "version"}, nil},
		// Custom registry, most insane form possible
		{"myregistry.local:5000/testing/test-image:version@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
			StructImageName{Long: "myregistry.local:5000/testing/test-image", Registry: "myregistry.local:5000", Short: "test-image", Tag: "version", Digest: "sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0"}, nil},
		// Test swarm image
		{"dockercloud/haproxy:1.6.7@sha256:8c4ed4049f55de49cbc8d03d057a5a7e8d609c264bb75b59a04470db1d1c5121",
			StructImageName{Long: "dockercloud/haproxy", Short: "haproxy", Tag: "1.6.7", Digest: "sha256:8c4ed4049f55de49cbc8d03d057a5a7e8d609c264bb75b59a04470db1d1c5121"}, nil},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.source), func(t *testing.T) {
			assert := assert.New(t)
			out, err := SplitImageName(tc.source)
			assert.Equal(tc.expected, out)

			if tc.err == nil {
				assert.Nil(err)
			} else {
				assert.NotNil(err)
				assert.Equal(tc.err.Error(), err.Error())
			}
		})
	}
}
