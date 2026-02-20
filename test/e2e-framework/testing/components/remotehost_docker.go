// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

// RemoteHostDocker represents an Agent running directly on a Host
type RemoteHostDocker struct {
	docker.ManagerOutput

	Client *client.Docker
}

var _ common.Initializable = (*RemoteHostDocker)(nil)

// Init is called by e2e test Suite after the component is provisioned.
func (d *RemoteHostDocker) Init(ctx common.Context) (err error) {
	d.Client, err = client.NewDocker(ctx.T(), d.ManagerOutput)
	return err
}
