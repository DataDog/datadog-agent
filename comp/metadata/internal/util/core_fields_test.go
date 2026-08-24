// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
)

func TestPopulateCoreFieldsInstallInfo(t *testing.T) {
	defer func() { installinfoGet = installinfo.Get }()

	conf := configmock.New(t)

	installinfoGet = func(model.Reader) (*installinfo.InstallInfo, error) {
		return nil, errors.New("some error")
	}
	data := map[string]interface{}{}
	PopulateCoreFields(data, conf, "agent", "")
	assert.Equal(t, "undefined", data["install_method_tool"])
	assert.Equal(t, "", data["install_method_tool_version"])
	assert.Equal(t, "", data["install_method_installer_version"])

	installinfoGet = func(model.Reader) (*installinfo.InstallInfo, error) {
		return &installinfo.InstallInfo{
			Tool:             "test_tool",
			ToolVersion:      "1.2.3",
			InstallerVersion: "4.5.6",
		}, nil
	}
	data = map[string]interface{}{}
	PopulateCoreFields(data, conf, "agent", "")
	assert.Equal(t, "test_tool", data["install_method_tool"])
	assert.Equal(t, "1.2.3", data["install_method_tool_version"])
	assert.Equal(t, "4.5.6", data["install_method_installer_version"])
}

func TestPopulateCoreFieldsFlavorAndHostnameSource(t *testing.T) {
	conf := configmock.New(t)

	// hostnameSource is omitted when empty and recorded when set; flavor is
	// injected verbatim.
	data := map[string]interface{}{}
	PopulateCoreFields(data, conf, "serverless-init", "")
	assert.Equal(t, "serverless-init", data["flavor"])
	_, ok := data["hostname_source"]
	assert.False(t, ok)

	data = map[string]interface{}{}
	PopulateCoreFields(data, conf, "agent", "gce")
	assert.Equal(t, "agent", data["flavor"])
	assert.Equal(t, "gce", data["hostname_source"])
}
