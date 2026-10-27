// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"errors"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/eudm"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var getEUDMHostname = eudm.Get

func fromEUDM(_ context.Context, _ string) (string, error) {
	if pkgconfigsetup.Datadog().GetString("infrastructure_mode") != constants.InfraModeEndUserDevice {
		return "", errors.New("EUDM hostname resolution only applies in end_user_device infrastructure mode")
	}
	name, err := getEUDMHostname()
	if err != nil && !errors.Is(err, errors.ErrUnsupported) {
		log.Warnf("Unable to determine EUDM device hostname; falling back to existing hostname resolution: %v", err)
	}
	return name, err
}
