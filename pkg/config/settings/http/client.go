// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http implements helpers for the runtime settings HTTP API
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	settingsComponent "github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

type client interface {
	DoGet(url string) (body []byte, e error)
	DoPost(url string, contentType string, body io.Reader) (resp []byte, e error)
}

type httpClient struct {
	c             *http.Client
	clientOptions ClientOptions
}

func (c *httpClient) DoGet(url string) (body []byte, e error) {
	ctx := context.Background()

	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return body, e
	}
	if c.clientOptions.CloseConnection == CloseConnection {
		req.Close = true
	}

	r, e := c.c.Do(req)
	if e != nil {
		return body, e
	}
	body, e = io.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return body, e
	}
	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

func (c *httpClient) DoPost(url string, contentType string, body io.Reader) (resp []byte, e error) {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return resp, e
	}

	req.Header.Set("Content-Type", contentType)

	r, e := c.c.Do(req)
	if e != nil {
		return resp, e
	}
	resp, e = io.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return resp, e
	}
	if r.StatusCode >= 400 {
		return resp, errors.New(string(resp))
	}
	return resp, nil
}

type httpsClient struct {
	c             ipc.HTTPClient
	clientOptions []ipc.RequestOption
}

func (c *httpsClient) DoGet(url string) (body []byte, e error) {
	return c.c.Get(url, ipchttp.WithLeaveConnectionOpen)
}

func (c *httpsClient) DoPost(url string, contentType string, body io.Reader) (resp []byte, e error) {
	return c.c.Post(url, contentType, body, ipchttp.WithLeaveConnectionOpen)
}

type runtimeSettingsClient struct {
	c                 client
	baseURL           string
	targetProcessName string
}

// NewHTTPClient returns a client setup to interact with the standard runtime settings HTTP API
func NewHTTPClient(c *http.Client, baseURL string, targetProcessName string, clientOptions ClientOptions) settings.Client {

	innerClient := &httpClient{c, clientOptions}

	return &runtimeSettingsClient{innerClient, baseURL, targetProcessName}
}

// NewHTTPSClient returns a client setup to interact with the standard runtime settings HTTPS API, taking advantage of the auth component
func NewHTTPSClient(c ipc.HTTPClient, baseURL string, targetProcessName string, clientOptions ...ipc.RequestOption) settings.Client {

	innerClient := &httpsClient{c, clientOptions}

	return &runtimeSettingsClient{innerClient, baseURL, targetProcessName}
}

func (rc *runtimeSettingsClient) doGet(url string, formatError bool) (string, error) {
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

func (rc *runtimeSettingsClient) FullConfig() (string, error) {
	r, err := rc.doGet(rc.baseURL, true)
	if err != nil {
		return "", err
	}
	return string(r), nil
}

func (rc *runtimeSettingsClient) FullConfigBySource() (string, error) {
	r, err := rc.doGet(fmt.Sprintf("%s/by-source", rc.baseURL), true)
	if err != nil {
		return "", err
	}
	return string(r), nil
}

func (rc *runtimeSettingsClient) List() (map[string]settingsComponent.RuntimeSettingResponse, error) {
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

func (rc *runtimeSettingsClient) Get(key string) (interface{}, error) {
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

func (rc *runtimeSettingsClient) GetWithSources(key string) (map[string]interface{}, error) {
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

func (rc *runtimeSettingsClient) Set(key string, value string) (bool, error) {
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
