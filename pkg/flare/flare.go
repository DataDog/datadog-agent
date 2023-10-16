// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// SendFlare sends a flare and returns the message returned by the backend. This entry point is deprecated in favor of
// the 'Send' method of the flare component.
func SendFlare(archivePath string, caseID string, email string, source helpers.FlareSource) (string, error) {
	return helpers.SendTo(archivePath, caseID, email, config.Datadog.GetString("api_key"), utils.GetInfraEndpoint(config.Datadog), source)
}
