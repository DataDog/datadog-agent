// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rum

var OTLPAttributeToRUMPayloadKeyMapping = map[string]string{
	// _common-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/_common-schema.json)
	ServiceName:                           Service,
	ServiceVersion:                        Version,
	SessionId:                             SessionId,
	UserId:                                UsrId,
	UserFullName:                          UsrName,
	UserEmail:                             UsrEmail,
	UserHash:                              UsrAnonymousId,
	UserName:                              AccountName,
	DatadogFormatVersion:                  DDFormatVersion,
	DatadogSessionPlan:                    DDSessionPlan,
	DatadogSessionSessionPrecondition:     DDSessionSessionPrecondition,
	DatadogConfigurationSessionSampleRate: DDConfigurationSessionSampleRate,
	DatadogConfigurationSessionReplaySampleRate: DDConfigurationSessionReplaySampleRate,
	DatadogConfigurationProfilingSampleRate:     DDConfigurationProfilingSampleRate,
	DatadogBrowserSDKVersion:                    DDBrowserSDKVersion,
	DatadogSDKName:                              DDSDKName,

	// action-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/action-schema.json)
	DatadogActionPositionX:      DDActionPositionX,
	DatadogActionPositionY:      DDActionPositionY,
	DatadogActionTargetSelector: DDActionTargetSelector,
	DatadogActionTargetWidth:    DDActionTargetWidth,
	DatadogActionTargetHeight:   DDActionTargetHeight,
	DatadogActionNameSource:     DDActionNameSource,

	// error-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/error-schema.json)
	ErrorMessage: ErrorMessage,
	ErrorType:    ErrorType,

	// long_task-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/long_task-schema.json)
	DatadogDiscarded: DDDiscarded,
	DatadogProfiling: DDProfiling,

	// resource-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/resource-schema.json)
	DatadogSpanId:               DDSpanId,
	DatadogParentSpanId:         DDParentSpanId,
	DatadogTraceId:              DDTraceId,
	DatadogRulePSR:              DDRulePSR,
	DatadogProfilingStatus:      DDProfilingStatus,
	DatadogProfilingErrorReason: DDProfilingErrorReason,

	// view-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/view-schema.json)
	DatadogDocumentVersion:                                  DDDocumentVersion,
	DatadogPageStates:                                       DDPageStates,
	DatadogPageStatesState:                                  DDPageStatesState,
	DatadogPageStatesStartTime:                              DDPageStatesStartTime,
	DatadogReplayStatsRecordsCount:                          DDReplayStatsRecordsCount,
	DatadogReplayStatsSegmentsCount:                         DDReplayStatsSegmentsCount,
	DatadogReplayStatsSegmentsTotalRawSize:                  DDReplayStatsSegmentsTotalRawSize,
	DatadogCLSDevicePixelRatio:                              DDCLSDevicePixelRatio,
	DatadogConfigurationStartSessionReplayRecordingManually: DDConfigurationStartSessionReplayRecordingManually,

	// vital-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/vital-schema.json)
	DatadogVitalComputedValue: DDVitalComputedValue,
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
