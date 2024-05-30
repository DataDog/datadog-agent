// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestDump(t *testing.T) {

	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "ctr-id",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "ctr-name",
		},
		Image: wmdef.ContainerImage{
			Name: "ctr-image",
		},
		Runtime:       wmdef.ContainerRuntimeDocker,
		RuntimeFlavor: wmdef.ContainerRuntimeFlavorKata,
		EnvVars: map[string]string{
			"DD_SERVICE":  "my-svc",
			"DD_ENV":      "prod",
			"DD_VERSION":  "v1",
			"NOT_ALLOWED": "not-allowed",
		},
	}

	ctrToMerge := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "ctr-id",
		},
		EntityMeta: wmdef.EntityMeta{
			Labels: map[string]string{"foo": "bar"},
		},
		Image: wmdef.ContainerImage{
			Tag: "latest",
		},
		PID:        1,
		CgroupPath: "/default/ctr-id",
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: "source1",
			Entity: container,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: "source2",
			Entity: ctrToMerge,
		},
	})

	shortDump := s.Dump(false)
	expectedShort := wmdef.WorkloadDumpResponse{
		Entities: map[string]wmdef.WorkloadEntity{
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
RuntimeFlavor: kata
Running: false
----------- Resources -----------
`,
				},
			},
		},
	}

	assert.EqualValues(t, expectedShort, shortDump)

	verboseDump := s.Dump(true)
	expectedVerbose := wmdef.WorkloadDumpResponse{
		Entities: map[string]wmdef.WorkloadEntity{
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
Repo Digest: 
----------- Container Info -----------
Runtime: docker
RuntimeFlavor: kata
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
Cgroup path: 
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
Repo Digest: 
----------- Container Info -----------
Runtime: 
RuntimeFlavor: 
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
Cgroup path: /default/ctr-id
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
Repo Digest: 
----------- Container Info -----------
Runtime: docker
RuntimeFlavor: kata
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
Cgroup path: /default/ctr-id
`,
				},
			},
		},
	}

	assert.EqualValues(t, expectedVerbose, verboseDump)
}
