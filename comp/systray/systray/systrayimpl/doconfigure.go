// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package systrayimpl

import (
	"fmt"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
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

	guiPort := s.config.GetString("GUI_port")
	if guiPort == "-1" {
		return fmt.Errorf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	endpoint, err := apiutil.NewIPCEndpoint(s.config, "/agent/gui/intent")
	if err != nil {
		return err
	}

	intentToken, err := endpoint.DoGet()
	if err != nil {
		return err
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://localhost:" + guiPort + "/auth?intent=" + string(intentToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: %s", err.Error())
	}

	s.log.Debugf("GUI opened at localhost:%s\n", guiPort)
	return nil
}
