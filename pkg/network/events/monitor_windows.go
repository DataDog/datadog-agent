// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events handles process events
package events

import (
	"os"
	"path/filepath"
	"time"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/iisconfig"
)

func getProcessStartTime(ev *model.Event) time.Time {
	if ev.GetEventType() == model.ExecEventType {
		return ev.GetProcessExecTime()
	}
	// windows does not have a fork event.
	return time.Time{}
}

func makeTagsSlice(apmtags iisconfig.APMTags) []*intern.Value {
	tags := make([]*intern.Value, 0, 3)
	if len(apmtags.DDService) > 0 {
		tags = append(tags, intern.GetByString("service"+":"+apmtags.DDService))
	}
	if len(apmtags.DDEnv) > 0 {
		tags = append(tags, intern.GetByString("env"+":"+apmtags.DDEnv))
	}
	if len(apmtags.DDVersion) > 0 {
		tags = append(tags, intern.GetByString("version"+":"+apmtags.DDVersion))
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}
func getAPMTags(filename string) []*intern.Value {

	dir := filepath.Dir(filename)
	if dir == "" {
		return nil
	}

	// see if there's an app.config in the directory
	appConfig := filepath.Join(dir, "app.config")
	ddJSON := filepath.Join(dir, "datadog.json")
	if _, err := os.Stat(appConfig); err == nil {

		appcfg, err := iisconfig.ReadDotNetConfig(appConfig)
		if err == nil {
			found := makeTagsSlice(appcfg)
			if len(found) > 0 {
				return found
			}
		}
	}
	// see if there's a datadog.json
	if _, err := os.Stat(ddJSON); err == nil {

		appcfg, err := iisconfig.ReadDatadogJSON(ddJSON)
		if err == nil {
			if err == nil {
				found := makeTagsSlice(appcfg)
				if len(found) > 0 {
					return found
				}
			}
		}
	}

	return nil
}
