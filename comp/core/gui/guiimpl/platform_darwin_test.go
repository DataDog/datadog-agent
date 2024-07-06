// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

const expectedBody = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Datadog Agent</title>
    <link rel="stylesheet" href="view/css/font-awesome.min.css">
    <link rel="stylesheet" href="view/css/codemirror.css">
    <link rel="stylesheet" href="view/css/stylesheet.css">
    <script src="view/js/polyfills.js"> </script>
    <script src="view/js/jquery-3.5.1.min.js"> </script>
    <script src="view/js/codemirror.js"> </script>
    <script src="view/js/yaml.js"> </script>
    <script src="view/js/javascript.js"> </script>
    <script src="view/js/purify.min.js"> </script>
</head>

<body>
  <div id="sidebar">
    <img id="logo" src="view/images/datadog_icon_white.svg">

    <ul class="navbar">
      <li class="nav_item multi active">
        <i class="fa fa-info fa-fw"> </i>&nbsp;
        Status
        <i class="fa fa-chevron-down fa-rotate-270"> </i>
        <div id="status_side_menu" class="side_menu">
          <a href="javascript:void(0)" onclick="loadStatus('general')" class="side_menu_item">General</a>
          <a href="javascript:void(0)" onclick="loadStatus('collector')" class="side_menu_item">Collector</a>
        </div>
      </li>
      <li id="log_button" class="nav_item">
        <i class="fa fa-book fa-fw"> </i>&nbsp;
        Log
      </li>
      <li id="settings_button" class="nav_item">
        <i class="fa fa-cog fa-fw"> </i>&nbsp;
        Settings
      </li>
      <li class="nav_item multi">
        <i class="fa fa-check fa-fw"> </i>&nbsp;
        Checks
        <i class="fa fa-chevron-down fa-rotate-270"> </i>
        <div id="checks_side_menu" class="side_menu">
          <a href="javascript:void(0)" onclick="loadManageChecks()" class="side_menu_item">Manage Checks</a>
          <a href="javascript:void(0)" onclick="seeRunningChecks()" class="side_menu_item">Checks Summary</a>
        </div>
      </li>
      <li id="flare_button" class="nav_item">
        <i class="fa fa-flag fa-fw"> </i>&nbsp;
        Flare
      </li>
    </ul>
  </div>
  <div class="top_bar">
    <div id="title">Datadog Agent Manager</div>
    <div id="agent_info">
      <div id="restart_status">Restart Agent to apply changes.</div>
      <div id="agent_status" class="disconnected">Not connected<br>to Agent</div>
      <div id="version">Version: </div>
      <div id="hostname">Hostname: </div>
    </div>
  </div>
  <div id="main">
    <div id="error" class="page">
      <div id="error_content"></div>
      <div id="logged_out">
        
<h3>Refreshing the Session</h3>
<p>Please ensure you access the Datadog Agent Manager with one of the following:</p>
<ul>
	<li>- through the Agent's menu bar extras icon by selecting "Open Web UI"</li>
	<li>- by running the following terminal command: "<code>datadog-agent launch-gui</code>"</li>
</ul>

<p>For more information, please visit: <u><a href="https://docs.datadoghq.com/agent/basic_agent_usage/osx">https://docs.datadoghq.com/agent/basic_agent_usage/osx</a></u></p>

<p>Note: If you would like to adjust the GUI session timeout, you can modify the <code>GUI_session_expiration</code> parameter in <code>datadog.yaml</code>

      </div>
    </div>
    <div id="tests" class="page"></div>
    <div id="general_status" class="page"></div>
    <div id="collector_status" class="page"></div>
    <div id="settings" class="page"></div>
    <div id="logs" class="page"></div>
    <div id="manage_checks" class="page">
      <div id="checks_description"></div>
      <div class="interface">
        <div class="left">
            <span class="dropdown">
              <select id="checks_dropdown">
                <option value="enabled">Edit Enabled Checks</option>
                <option value="add">Add a Check</option>
              </select>
            </span>
          <div class="list"></div>
        </div>
        <div class="right"></div>
      </div>
    </div>
    <div id="running_checks" class="page"></div>
    <div id="flare" class="page">
        <div id="flare_description"></div>
        <form class="flare_input center">
          <input type="number" id="ticket_num" placeholder="Ticket number, if you have one"/>
          <input type="email" id="email" placeholder="Email address"/>
          <input type="button" id="submit_flare" value="Submit"/>
        </form>
    </div>

  </div>
</body>
`

func TestRenderIndexPage(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(renderIndexPage)

	handler.ServeHTTP(rr, req)

	res := rr.Result()
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	assert.Equal(t, expectedBody, string(bodyBytes))
}
