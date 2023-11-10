// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usersessions holds model related to the user sessions resolver
package usersessions

import (
	"encoding/json"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
)

// PrepareK8SUserSessionContext prepares the input parameters forwarded to cws-instrumentation
func PrepareK8SUserSessionContext(userInfo *authenticationv1.UserInfo, cwsUserSessionDataMaxSize int) ([]byte, error) {
	userSessionCtx, err := json.Marshal(userInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall authenticationv1.UserInfo structure: %w", err)
	}
	if len(userSessionCtx) <= cwsUserSessionDataMaxSize {
		return userSessionCtx, nil
	}

	// try to remove the extra field
	info := *userInfo
	info.Extra = nil

	userSessionCtx, err = json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall authenticationv1.UserInfo structure: %w", err)
	}
	if len(userSessionCtx) <= cwsUserSessionDataMaxSize {
		return userSessionCtx, nil
	}

	// try to remove the groups field
	info.Groups = nil
	userSessionCtx, err = json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall authenticationv1.UserInfo structure: %w", err)
	}

	if len(userSessionCtx) <= cwsUserSessionDataMaxSize {
		return userSessionCtx, nil
	}
	return nil, fmt.Errorf("authenticationv1.UserInfo structure too big (%d), ignoring instrumentation", len(userSessionCtx))
}
