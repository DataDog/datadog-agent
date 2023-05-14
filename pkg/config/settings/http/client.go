// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

type runtimeSettingsHTTPClient struct {
	c                 *http.Client
	baseURL           string
	targetProcessName string
}

// NewClient returns a client setup to interact with the standard runtime settings HTTP API
func NewClient(c *http.Client, baseURL string, targetProcessName string) settings.Client {
	return &runtimeSettingsHTTPClient{c, baseURL, targetProcessName}
}

func (rc *runtimeSettingsHTTPClient) FullConfig() (string, error) {
	r, err := util.DoGet(rc.c, rc.baseURL, util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return "", fmt.Errorf(e)
		}

		return "", fmt.Errorf("Could not reach %s: %v \nMake sure the %s is running before requesting the runtime configuration and contact support if you continue having issues", rc.targetProcessName, err, rc.targetProcessName)
	}

	return string(r), nil
}

func (rc *runtimeSettingsHTTPClient) List() (map[string]settings.RuntimeSettingResponse, error) {
	r, err := util.DoGet(rc.c, fmt.Sprintf("%s/%s", rc.baseURL, "list-runtime"), util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return nil, fmt.Errorf(e)
		}
		return nil, err
	}
	var settingsList = make(map[string]settings.RuntimeSettingResponse)
	err = json.Unmarshal(r, &settingsList)
	if err != nil {
		return nil, err
	}

	return settingsList, nil
}

func (rc *runtimeSettingsHTTPClient) Get(key string) (interface{}, error) {
	r, err := util.DoGet(rc.c, fmt.Sprintf("%s/%s", rc.baseURL, key), util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return nil, fmt.Errorf(e)
		}
		return nil, err
	}

	var setting = make(map[string]interface{})
	err = json.Unmarshal(r, &setting)
	if err != nil {
		return nil, err
	}
	if value, found := setting["value"]; found {
		return value, nil
	}
	return nil, fmt.Errorf("unable to get value for this setting: %v", key)
}

func (rc *runtimeSettingsHTTPClient) Set(key string, value string) (bool, error) {
	settingsList, err := rc.List()
	if err != nil {
		return false, err
	}

	body := fmt.Sprintf("value=%s", html.EscapeString(value))
	r, err := util.DoPost(rc.c, fmt.Sprintf("%s/%s", rc.baseURL, key), "application/x-www-form-urlencoded", bytes.NewBuffer([]byte(body)))
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return false, fmt.Errorf(e)
		}
		return false, err
	}

	hidden := false
	if setting, ok := settingsList[key]; ok {
		hidden = setting.Hidden
	}
	return hidden, nil
}
