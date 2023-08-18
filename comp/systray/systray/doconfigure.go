// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package systray

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func onConfigure(s *systray) {
	// seems like a waste.  However, the handler function doesn't expect an error code.
	// this just eats the error code.
	err := doConfigure(s)
	if err != nil {
		s.log.Warnf("Failed to launch gui %v", err)
	}
	return
}
func doConfigure(s *systray) error {

	guiPort := s.config.GetString("GUI_port")
	if guiPort == "-1" {
		return fmt.Errorf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	// Read the authentication token: can only be done if user can read from datadog.yaml
	authToken, err := security.FetchAuthToken()
	if err != nil {
		return err
	}

	// Get the CSRF token from the agent
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/gui/csrf-token", ipcAddress, s.config.GetInt("cmd_port"))
	err = util.SetAuthToken()
	if err != nil {
		return err
	}

	csrfToken, err := util.DoGet(c, urlstr, util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		err = json.Unmarshal(csrfToken, &errMap)
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}
		return fmt.Errorf("Could not reach agent: %v \nMake sure the agent is running before attempting to open the GUI", err)
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://127.0.0.1:" + guiPort + "/authenticate?authToken=" + string(authToken) + "&csrf=" + string(csrfToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: " + err.Error())
	}

	s.log.Debugf("GUI opened at 127.0.0.1:" + guiPort + "\n")
	return nil
}
