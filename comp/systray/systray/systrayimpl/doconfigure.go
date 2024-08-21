// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package systrayimpl

import (
	"fmt"
	"net"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/system"
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

	// 'http://localhost' is preferred over 'http://127.0.0.1' due to Internet Explorer behavior.
	// Internet Explorer High Security Level does not support setting cookies via HTTP Header response.
	// By default, 'http://localhost' is categorized as an "intranet" website, which is considered safer and allowed to use cookies. This is not the case for 'http://127.0.0.1'.
	guiHost, err := system.IsLocalAddress(s.config.GetString("GUI_host"))
	if err != nil {
		return fmt.Errorf("GUI server host is not a local address: %s", err)
	}

	endpoint, err := apiutil.NewIPCEndpoint(s.config, "/agent/gui/intent")
	if err != nil {
		return err
	}

	intentToken, err := endpoint.DoGet()
	if err != nil {
		return err
	}

	guiAddress := net.JoinHostPort(guiHost, guiPort)

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://" + guiAddress + "/auth?intent=" + string(intentToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: %s", err.Error())
	}

	s.log.Debugf("GUI opened at %s\n", guiPort)
	return nil
}
