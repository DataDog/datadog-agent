// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package security

import (
	"fmt"
	"io/ioutil"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	acl "github.com/hectane/go-acl"
	"golang.org/x/sys/windows"
)

// lookupUsernameAndDomain obtains the username and domain for usid.
func lookupUsernameAndDomain(usid *syscall.SID) (username, domain string, e error) {
	username, domain, t, e := usid.LookupAccount("")
	if e != nil {
		return "", "", e
	}
	if t != syscall.SidTypeUser {
		return "", "", fmt.Errorf("user: should be user account type, not %d", t)
	}
	return username, domain, nil
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token, tokenPath string) error {
	err = ioutil.WriteFile(tokenPath, []byte(token), 0700)
	if err != nil {
		return err
	}

	if perms, err := filesystem.NewPermission(); err != nil {
		return err
	}

	if err := perms.RestrictAccessToUser(tokenPath); err != nil {
		log.Infof("Wrote auth token acl")
	} else {
		log.Errorf("Failed to write auth token acl %s", err)
		return err
	}
}
