// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

var otlpAttributeToRUMPayloadKeyMapping = map[string]string{
	// _common-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/_common-schema.json)
	serviceName:    service,
	serviceVersion: version,
	sessionID:      session,
	userID:         usrID,
	userFullName:   usrName,
	userEmail:      usrEmail,
	userHash:       usrAnonymousID,
	userName:       accountName,

	// error-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/error-schema.json)
	errorMessage: errorMessage,
	errorType:    errorType,
}

var rumPayloadKeyToOTLPAttributeMapping = map[string]string{
	// _common-schema.json
	service:        serviceName,
	version:        serviceVersion,
	sessionID:      session,
	usrID:          userID,
	usrName:        userFullName,
	usrEmail:       userEmail,
	usrAnonymousID: userHash,
	accountName:    userName,

	// error-schema.json
	errorMessage: errorMessage,
	errorType:    errorType,
}
