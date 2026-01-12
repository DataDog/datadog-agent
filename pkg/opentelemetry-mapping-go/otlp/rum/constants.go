// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

const (
	instrumentationScopeName = "datadog.rum-browser-sdk"
	typeKey                  = "type"

	// _common-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/_common-schema.json)
	serviceName    = "service.name"
	serviceVersion = "service.version"
	sessionID      = "session.id"
	userID         = "user.id"
	userFullName   = "user.full_name"
	userEmail      = "user.email"
	userHash       = "user.hash"
	userName       = "user.name"

	service        = "service"
	session        = "session"
	version        = "version"
	usrID          = "usr.id"
	usrName        = "usr.name"
	usrEmail       = "usr.email"
	usrAnonymousID = "usr.anonymous_id"
	accountName    = "account.name"

	// error-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/error-schema.json)
	errorMessage = "error.message"
	errorType    = "error.type"
)
