// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

var OTLPAttributeToRUMPayloadKeyMapping = map[string]string{
	// _common-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/_common-schema.json)
	ServiceName:    Service,
	ServiceVersion: Version,
	SessionId:      SessionId,
	UserId:         UsrId,
	UserFullName:   UsrName,
	UserEmail:      UsrEmail,
	UserHash:       UsrAnonymousId,
	UserName:       AccountName,

	// error-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/error-schema.json)
	ErrorMessage: ErrorMessage,
	ErrorType:    ErrorType,
}

var RUMPayloadKeyToOTLPAttributeMapping = map[string]string{
	// _common-schema.json
	Service:        ServiceName,
	Version:        ServiceVersion,
	SessionId:      SessionId,
	UsrId:          UserId,
	UsrName:        UserFullName,
	UsrEmail:       UserEmail,
	UsrAnonymousId: UserHash,
	AccountName:    UserName,

	// error-schema.json
	ErrorMessage: ErrorMessage,
	ErrorType:    ErrorType,
}
