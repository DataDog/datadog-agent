// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package systrayimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func onConfigure(s *systrayImpl) {
	// seems like a waste.  However, the handler function doesn't expect an error code.
	// this just eats the error code.
	err := doConfigure(s)
	if err != nil {
		s.log.Warnf("Failed to launch gui %v", err)
	}
}

func doConfigure(s *systrayImpl) error {
	// Start the agent service if it's not running
	onStart(s)

	// If the agent service was not already running, wait for it to be running
	ctx, cancel := context.WithTimeout(context.Background(), winutil.DefaultServiceCommandTimeout*time.Second)
	defer cancel()

	if err := winutil.WaitForState(ctx, common.ServiceName, svc.Running); err != nil {
		return fmt.Errorf("agent service failed to start within timeout: %v", err)
	}

	guiPort := s.config.GetString("GUI_port")
	if guiPort == "-1" {
		return errors.New("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	guiAddress, err := pkgconfighelper.GetGUIAddress(s.config)
	if err != nil {
		return fmt.Errorf("GUI server host is not a local address: %s", err)
	}

	endpoint, err := s.client.NewIPCEndpoint("/agent/gui/intent")
	if err != nil {
		return err
	}

	intentToken, err := endpoint.DoGet()
	if err != nil {
		return err
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://" + guiAddress + "/auth?intent=" + string(intentToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: %s", err.Error())
	}

	s.log.Debugf("GUI opened at %s\n", guiAddress)
	return nil
}
