// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/docker"
	dclient "github.com/docker/docker/client"
)

// RemoteHostDocker represents an Agent running directly on a Host
type RemoteHostDocker struct {
	docker.ManagerOutput

	Client *client.Docker
}

var _ e2e.Initializable = (*RemoteHostDocker)(nil)

// Init is called by e2e test Suite after the component is provisioned.
func (d *RemoteHostDocker) Init(ctx e2e.Context) (err error) {
	d.Client, err = client.NewDocker(ctx.T(), d.ManagerOutput)
	return err
}

// GetClient returns the Docker client for the host
func (d *RemoteHostDocker) GetClient() *dclient.Client {
	return d.Client.GetClient()
}
