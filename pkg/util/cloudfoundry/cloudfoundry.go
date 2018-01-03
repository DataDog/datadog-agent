// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package cloudfoudry

import (
	"os"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// GetHostAlias returns the host alias from Cloud Foundry
func GetHostAlias() (string, error) {
	if !config.Datadog.GetBool("cloud_foundry") {
		log.Info("cloud_foundry is not enable in the conf: not cloudfoudry host alias")
		return "", nil
	}

	boshID := config.Datadog.GetString("bosh_id")
	if boshID != "" {
		return boshID, nil
	}

	hostname, _ := os.Hostname()
	return util.Fqdn(hostname), nil
}
