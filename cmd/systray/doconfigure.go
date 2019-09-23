// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build windows

package main

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func canConfigure() bool {
	if _, err := security.FetchAuthToken(); err != nil {
		return false
	}
	return true
}
func onConfigure() {
	// seems like a waste.  However, the handler function doesn't expect an error code.
	// this just eates the error code.
	err := doConfigure()
	if err != nil {
		log.Warnf("Failed to launch gui %v", err)
	}
	return
}
func doConfigure() error {

	err := common.SetupConfigWithoutSecrets("")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	guiPort := config.Datadog.GetString("GUI_port")
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
	urlstr := fmt.Sprintf("https://%v:%v/agent/gui/csrf-token", config.Datadog.GetString("bind_ipc"), config.Datadog.GetInt("cmd_port"))
	err = util.SetAuthToken()
	if err != nil {
		return err
	}

	csrfToken, err := util.DoGet(c, urlstr)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(csrfToken, errMap)
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}
		return fmt.Errorf("Could not reach agent: %v \nMake sure the agent is running before attempting to open the GUI", err)
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://127.0.0.1:" + guiPort + "/authenticate?authToken=" + string(authToken) + ";csrf=" + string(csrfToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: " + err.Error())
	}

	log.Debugf("GUI opened at 127.0.0.1:" + guiPort + "\n")
	return nil
}
