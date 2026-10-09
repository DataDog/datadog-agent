// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"errors"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/eudm"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var getEUDMHostname = eudm.Get
var eudmSupported = eudm.Supported

func fromEUDM(_ context.Context, _ string) (string, error) {
	if pkgconfigsetup.Datadog().GetString("infrastructure_mode") != "end_user_device" || !eudmSupported() {
		return "", errors.New("EUDM hostname resolution is not enabled on this host")
	}
	name, err := getEUDMHostname()
	if err != nil {
		log.Warnf("Unable to determine EUDM device hostname; falling back to existing hostname resolution: %v", err)
	}
	return name, err
}
