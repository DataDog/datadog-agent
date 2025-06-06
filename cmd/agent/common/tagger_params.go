// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"crypto/tls"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// DualTaggerParams returns the params use inside the main agent
func DualTaggerParams() (tagger.DualParams, tagger.RemoteParams) {
	return tagger.DualParams{
			UseRemote: func(c config.Component) bool {
				return pkgconfigsetup.IsCLCRunner(c) && c.GetBool("clc_runner_remote_tagger_enabled")
			},
		}, tagger.RemoteParams{
			RemoteTarget: func(config.Component) (string, error) {
				target, err := utils.GetClusterAgentEndpoint()
				if err != nil {
					return "", err
				}
				return strings.TrimPrefix(target, "https://"), nil
			},
			RemoteFilter: types.NewFilterBuilder().Exclude(types.KubernetesPodUID).Build(types.HighCardinality),

			// TODO IPC: we don't have IPC certificate supply chain between CLC and Cluster Agent yet
			// So we need to skip the verification
			OverrideTLSConfig:       &tls.Config{InsecureSkipVerify: true},
			OverrideAuthTokenGetter: security.GetClusterAgentAuthToken,
		}
}
