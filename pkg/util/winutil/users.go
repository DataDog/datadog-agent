// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.
//go:build windows
// +build windows

package winutil

import (
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetSidFromUser grabs and returns the windows SID for the current user or an error.
// The *SID returned does not need to be freed by the caller.
func GetSidFromUser() (*windows.SID, error) {
	log.Infof("Getting sidstring from user")
	tok, e := syscall.OpenCurrentProcessToken()
	if e != nil {
		log.Warnf("Couldn't get process token %v", e)
		return nil, e
	}
	defer tok.Close()

	user, e := tok.GetTokenUser()
	if e != nil {
		log.Warnf("Couldn't get token user %v", e)
		return nil, e
	}

	sidString, e := user.User.Sid.String()
	if e != nil {
		log.Warnf("Couldn't get user sid string %v", e)
		return nil, e
	}

	return windows.StringToSid(sidString)
}
