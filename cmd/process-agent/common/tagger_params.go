// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common provides a set of common functions used on different commands
package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// DualTaggerParams returns the params use inside the main agent
func DualTaggerParams() (tagger.DualParams, tagger.Params, tagger.RemoteParams) {
	return tagger.DualParams{
			UseRemote: func(c config.Component) bool {
				return c.GetBool("process_config.remote_tagger") ||
					// If the agent is running in ECS or ECS Fargate and the ECS task collection is enabled, use the remote tagger
					// as remote tagger can return more tags than the local tagger.
					((env.IsECS() || env.IsECSFargate()) && c.GetBool("ecs_task_collection_enabled"))
			},
		}, tagger.Params{}, tagger.RemoteParams{
			RemoteTarget: func(c config.Component) (string, error) {
				return fmt.Sprintf(":%v", c.GetInt("cmd_port")), nil
			},
			RemoteTokenFetcher: func(c config.Component) func() (string, error) {
				return func() (string, error) {
					return security.FetchAuthToken(c)
				}
			},
			RemoteFilter: types.NewMatchAllFilter(),
		}
}
