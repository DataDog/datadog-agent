// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http implements helpers for the runtime settings HTTP API
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/secureclient"
	settingsComponent "github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

type httpClient interface {
	DoGet(url string) (body []byte, e error)
	DoPost(url string, contentType string, body io.Reader) (resp []byte, e error)
}

type insecureHTTPClient struct {
	c             *http.Client
	clientOptions ClientOptions
}

func (c *insecureHTTPClient) DoGet(url string) (body []byte, e error) {
	return util.DoGet(c.c, url, util.CloseConnection)
}

func (c *insecureHTTPClient) DoPost(url string, contentType string, body io.Reader) (resp []byte, e error) {
	return util.DoPost(c.c, url, contentType, body)
}

type secureHTTPClient struct {
	c             authtoken.SecureClient
	clientOptions []authtoken.RequestOption
}

func (c *secureHTTPClient) DoGet(url string) (body []byte, e error) {
	return c.c.Get(url, secureclient.WithLeaveConnectionOpen)
}

func (c *secureHTTPClient) DoPost(url string, contentType string, body io.Reader) (resp []byte, e error) {
	return c.c.Post(url, contentType, body, secureclient.WithLeaveConnectionOpen)
}

type runtimeSettingsHTTPClient struct {
	c                 httpClient
	baseURL           string
	targetProcessName string
}

// NewClient returns a client setup to interact with the standard runtime settings HTTP API
func NewClient(c *http.Client, baseURL string, targetProcessName string, clientOptions ClientOptions) settings.Client {

	innerClient := &insecureHTTPClient{c, clientOptions}

	return &runtimeSettingsHTTPClient{innerClient, baseURL, targetProcessName}
}

// NewSecureClient returns a client setup to interact with the standard runtime settings HTTPS API, taking advantage of the auth component
func NewSecureClient(c authtoken.SecureClient, baseURL string, targetProcessName string, clientOptions ...authtoken.RequestOption) settings.Client {

	innerClient := &secureHTTPClient{c, clientOptions}

	return &runtimeSettingsHTTPClient{innerClient, baseURL, targetProcessName}
}

func (rc *runtimeSettingsHTTPClient) doGet(url string, formatError bool) (string, error) {
	var r []byte
	var err error

	r, err = rc.c.DoGet(url)

	if err != nil {
		errMap := make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return "", errors.New(e)
		}
		if formatError {
			return "", fmt.Errorf("Could not reach %s: %v \nMake sure the %s is running before requesting the runtime configuration and contact support if you continue having issues", rc.targetProcessName, err, rc.targetProcessName)
		}
		return "", err
	}
	return string(r), nil
}

func (rc *runtimeSettingsHTTPClient) FullConfig() (string, error) {
	r, err := rc.doGet(rc.baseURL, true)
	if err != nil {
		return "", err
	}
	return string(r), nil
}

func (rc *runtimeSettingsHTTPClient) FullConfigBySource() (string, error) {
	r, err := rc.doGet(fmt.Sprintf("%s/by-source", rc.baseURL), true)
	if err != nil {
		return "", err
	}
	return string(r), nil
}

func (rc *runtimeSettingsHTTPClient) List() (map[string]settingsComponent.RuntimeSettingResponse, error) {
	r, err := rc.doGet(fmt.Sprintf("%s/list-runtime", rc.baseURL), false)
	if err != nil {
		return nil, err
	}
	settingsList := make(map[string]settingsComponent.RuntimeSettingResponse)
	err = json.Unmarshal([]byte(r), &settingsList)
	if err != nil {
		return nil, err
	}

	return settingsList, nil
}

func (rc *runtimeSettingsHTTPClient) Get(key string) (interface{}, error) {
	r, err := rc.doGet(fmt.Sprintf("%s/%s", rc.baseURL, key), false)
	if err != nil {
		return nil, err
	}

	setting := make(map[string]interface{})
	err = json.Unmarshal([]byte(r), &setting)
	if err != nil {
		return nil, err
	}
	if value, found := setting["value"]; found {
		return value, nil
	}
	return nil, fmt.Errorf("unable to get value for this setting: %v", key)
}

func (rc *runtimeSettingsHTTPClient) GetWithSources(key string) (map[string]interface{}, error) {
	r, err := rc.doGet(fmt.Sprintf("%s/%s?sources=true", rc.baseURL, key), false)
	if err != nil {
		return nil, err
	}

	setting := make(map[string]interface{})
	err = json.Unmarshal([]byte(r), &setting)
	if err != nil {
		return nil, err
	}

	if _, found := setting["value"]; !found {
		return nil, fmt.Errorf("unable to get value for this setting: %v", key)
	}

	if _, found := setting["sources_value"]; !found {
		return nil, fmt.Errorf("unable to get sources value for this setting: %v", key)
	}

	return setting, nil
}

func (rc *runtimeSettingsHTTPClient) Set(key string, value string) (bool, error) {
	settingsList, err := rc.List()
	if err != nil {
		return false, err
	}

	body := fmt.Sprintf("value=%s", html.EscapeString(value))
	r, err := rc.c.DoPost(fmt.Sprintf("%s/%s", rc.baseURL, key), "application/x-www-form-urlencoded", bytes.NewBuffer([]byte(body)))
	if err != nil {
		errMap := make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return false, errors.New(e)
		}
		return false, err
	}

	hidden := false
	if setting, ok := settingsList[key]; ok {
		hidden = setting.Hidden
	}
	return hidden, nil
}
