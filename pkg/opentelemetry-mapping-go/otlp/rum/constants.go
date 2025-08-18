// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

const (
	InstrumentationScopeName = "datadog.rum-browser-sdk"
	Type                     = "type"

	// _common-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/_common-schema.json)
	ServiceName    = "service.name"
	ServiceVersion = "service.version"
	SessionId      = "session.id"
	UserId         = "user.id"
	UserFullName   = "user.full_name"
	UserEmail      = "user.email"
	UserHash       = "user.hash"
	UserName       = "user.name"

	Service        = "service"
	Session        = "session"
	Version        = "version"
	UsrId          = "usr.id"
	UsrName        = "usr.name"
	UsrEmail       = "usr.email"
	UsrAnonymousId = "usr.anonymous_id"
	AccountName    = "account.name"

	// error-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/error-schema.json)
	ErrorMessage = "error.message"
	ErrorType    = "error.type"
)
