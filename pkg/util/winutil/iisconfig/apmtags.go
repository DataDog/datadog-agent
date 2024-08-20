// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

// Package iisconfig manages iis configuration
package iisconfig

/*
the file datadog.json can be located anywhere; it is path-relative to a .net application
give the path name, read the json and return it as a map of string/string
*/

import (
	"encoding/json"
	"encoding/xml"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type APMTags struct {
	DDService string
	DDEnv     string
	DDVersion string
}

// ReadDatadogJSON reads a datadog.json file and returns the APM tags
func ReadDatadogJSON(datadogJSONPath string) (APMTags, error) {
	var datadogJSON map[string]string
	var apmtags APMTags

	file, err := os.Open(datadogJSONPath)
	if err != nil {
		return apmtags, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&datadogJSON)
	if err != nil {
		return apmtags, err
	}
	apmtags.DDService = datadogJSON["DD_SERVICE"]
	apmtags.DDEnv = datadogJSON["DD_ENV"]
	apmtags.DDVersion = datadogJSON["DD_VERSION"]
	return apmtags, nil
}

type iisAppSetting struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}
type iisAppSettings struct {
	XMLName xml.Name        `xml:"appSettings"`
	Adds    []iisAppSetting `xml:"add"`
}

type appConfiguration struct {
	XMLName     xml.Name `xml:"configuration"`
	AppSettings iisAppSettings
}

var (
	errorlogcount = 0
)

// ReadDotNetConfig reads an iis config file(xml) and returns the APM tags
func ReadDotNetConfig(cfgpath string) (APMTags, error) { //(APMTags, error) {
	var newcfg appConfiguration
	var apmtags APMTags
	var chasedatadogJSON string
	f, err := os.ReadFile(cfgpath)
	if err != nil {
		return apmtags, err
	}
	err = xml.Unmarshal(f, &newcfg)
	if err != nil {
		return apmtags, err
	}
	for _, setting := range newcfg.AppSettings.Adds {
		switch setting.Key {
		case "DD_SERVICE":
			apmtags.DDService = setting.Value
		case "DD_ENV":
			apmtags.DDEnv = setting.Value
		case "DD_VERSION":
			apmtags.DDVersion = setting.Value
		case "DD_TRACE_CONFIG_FILE":
			chasedatadogJSON = setting.Value
		}
	}
	if len(chasedatadogJSON) > 0 {
		ddjson, err := ReadDatadogJSON(chasedatadogJSON)
		if err == nil {
			if len(ddjson.DDService) > 0 {
				apmtags.DDService = ddjson.DDService
			}
			if len(ddjson.DDEnv) > 0 {
				apmtags.DDEnv = ddjson.DDEnv
			}
			if len(ddjson.DDVersion) > 0 {
				apmtags.DDVersion = ddjson.DDVersion
			}
		} else {
			// only log every 1000 occurrences because if this is misconfigured, it could flood the log
			if errorlogcount%1000 == 0 {
				log.Warnf("Error reading configured datadog.json file %s: %v", chasedatadogJSON, err)
			}
			errorlogcount++
		}
	}
	return apmtags, nil
}
