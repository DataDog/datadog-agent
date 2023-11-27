// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDump(t *testing.T) {
	s := newTestStore()

	container := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "ctr-id",
		},
		EntityMeta: EntityMeta{
			Name: "ctr-name",
		},
		Image: ContainerImage{
			Name: "ctr-image",
		},
		Runtime: ContainerRuntimeDocker,
		EnvVars: map[string]string{
			"DD_SERVICE":  "my-svc",
			"DD_ENV":      "prod",
			"DD_VERSION":  "v1",
			"NOT_ALLOWED": "not-allowed",
		},
	}

	ctrToMerge := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "ctr-id",
		},
		EntityMeta: EntityMeta{
			Labels: map[string]string{"foo": "bar"},
		},
		Image: ContainerImage{
			Tag: "latest",
		},
		PID: 1,
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: "source1",
			Entity: container,
		},
		{
			Type:   EventTypeSet,
			Source: "source2",
			Entity: ctrToMerge,
		},
	})

	shortDump := s.Dump(false)
	expectedShort := WorkloadDumpResponse{
		Entities: map[string]WorkloadEntity{
			"container": {
				Infos: map[string]string{
					"sources(merged):[source1 source2] id: ctr-id": `----------- Entity ID -----------
Kind: container ID: ctr-id
----------- Entity Meta -----------
Name: ctr-name
Namespace: 
----------- Image -----------
Name: ctr-image
Tag: latest
----------- Container Info -----------
Runtime: docker
Running: false
----------- Resources -----------
`,
				},
			},
		},
	}

	assert.EqualValues(t, expectedShort, shortDump)

	verboseDump := s.Dump(true)
	expectedVerbose := WorkloadDumpResponse{
		Entities: map[string]WorkloadEntity{
			"container": {
				Infos: map[string]string{
					"source:source1 id: ctr-id": `----------- Entity ID -----------
Kind: container ID: ctr-id
----------- Entity Meta -----------
Name: ctr-name
Namespace: 
Annotations: 
Labels: 
----------- Image -----------
Name: ctr-image
Tag: 
ID: 
Raw Name: 
Short Name: 
----------- Container Info -----------
Runtime: docker
Running: false
Status: 
Health: 
Created At: 0001-01-01 00:00:00 +0000 UTC
Started At: 0001-01-01 00:00:00 +0000 UTC
Finished At: 0001-01-01 00:00:00 +0000 UTC
----------- Resources -----------
Allowed env variables: DD_SERVICE:my-svc DD_ENV:prod DD_VERSION:v1 
Hostname: 
Network IPs: 
PID: 0
`,
					"source:source2 id: ctr-id": `----------- Entity ID -----------
Kind: container ID: ctr-id
----------- Entity Meta -----------
Name: 
Namespace: 
Annotations: 
Labels: foo:bar 
----------- Image -----------
Name: 
Tag: latest
ID: 
Raw Name: 
Short Name: 
----------- Container Info -----------
Runtime: 
Running: false
Status: 
Health: 
Created At: 0001-01-01 00:00:00 +0000 UTC
Started At: 0001-01-01 00:00:00 +0000 UTC
Finished At: 0001-01-01 00:00:00 +0000 UTC
----------- Resources -----------
Allowed env variables: 
Hostname: 
Network IPs: 
PID: 1
`,
					"sources(merged):[source1 source2] id: ctr-id": `----------- Entity ID -----------
Kind: container ID: ctr-id
----------- Entity Meta -----------
Name: ctr-name
Namespace: 
Annotations: 
Labels: foo:bar 
----------- Image -----------
Name: ctr-image
Tag: latest
ID: 
Raw Name: 
Short Name: 
----------- Container Info -----------
Runtime: docker
Running: false
Status: 
Health: 
Created At: 0001-01-01 00:00:00 +0000 UTC
Started At: 0001-01-01 00:00:00 +0000 UTC
Finished At: 0001-01-01 00:00:00 +0000 UTC
----------- Resources -----------
Allowed env variables: DD_SERVICE:my-svc DD_ENV:prod DD_VERSION:v1 
Hostname: 
Network IPs: 
PID: 1
`,
				},
			},
		},
	}

	assert.EqualValues(t, expectedVerbose, verboseDump)
}
