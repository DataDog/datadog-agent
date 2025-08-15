// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rum

const (
	InstrumentationScopeName = "datadog.rum-browser-sdk"

	// _common-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/_common-schema.json)
	ServiceName                                 = "service.name"
	ServiceVersion                              = "service.version"
	SessionId                                   = "session.id"
	UserId                                      = "user.id"
	UserFullName                                = "user.full_name"
	UserEmail                                   = "user.email"
	UserHash                                    = "user.hash"
	UserName                                    = "user.name"
	DatadogFormatVersion                        = "datadog.format_version"
	DatadogSessionPlan                          = "datadog.session.plan"
	DatadogSessionSessionPrecondition           = "datadog.session.session_precondition"
	DatadogConfigurationSessionSampleRate       = "datadog.configuration.session_sample_rate"
	DatadogConfigurationSessionReplaySampleRate = "datadog.configuration.session_replay_sample_rate"
	DatadogConfigurationProfilingSampleRate     = "datadog.configuration.profiling_sample_rate"
	DatadogBrowserSDKVersion                    = "datadog.browser_sdk_version"
	DatadogSDKName                              = "datadog.sdk_name"

	Service                                = "service"
	Session                                = "session"
	Version                                = "version"
	UsrId                                  = "usr.id"
	UsrName                                = "usr.name"
	UsrEmail                               = "usr.email"
	UsrAnonymousId                         = "usr.anonymous_id"
	AccountName                            = "account.name"
	DDFormatVersion                        = "_dd.format_version"
	DDSessionPlan                          = "_dd.session.plan"
	DDSessionSessionPrecondition           = "_dd.session.session_precondition"
	DDConfigurationSessionSampleRate       = "_dd.configuration.session_sample_rate"
	DDConfigurationSessionReplaySampleRate = "_dd.configuration.session_replay_sample_rate"
	DDConfigurationProfilingSampleRate     = "_dd.configuration.profiling_sample_rate"
	DDBrowserSDKVersion                    = "_dd.browser_sdk_version"
	DDSDKName                              = "_dd.sdk_name"
	Type                                   = "type"

	// action-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/action-schema.json)
	DatadogActionPositionX      = "datadog.action.position.x"
	DatadogActionPositionY      = "datadog.action.position.y"
	DatadogActionTargetSelector = "datadog.action.target.selector"
	DatadogActionTargetWidth    = "datadog.action.target.width"
	DatadogActionTargetHeight   = "datadog.action.target.height"
	DatadogActionNameSource     = "datadog.action.name_source"

	DDActionPositionX      = "_dd.action.position.x"
	DDActionPositionY      = "_dd.action.position.y"
	DDActionTargetSelector = "_dd.action.target.selector"
	DDActionTargetWidth    = "_dd.action.target.width"
	DDActionTargetHeight   = "_dd.action.target.height"
	DDActionNameSource     = "_dd.action.name_source"

	// error-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/error-schema.json)
	ErrorMessage = "error.message"
	ErrorType    = "error.type"

	// long_task-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/long_task-schema.json)
	DatadogDiscarded = "datadog.discarded"
	DatadogProfiling = "datadog.profiling"

	DDDiscarded = "_dd.discarded"
	DDProfiling = "_dd.profiling"

	// resource-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/resource-schema.json)
	DatadogSpanId               = "datadog.span_id"
	DatadogParentSpanId         = "datadog.parent_span_id"
	DatadogTraceId              = "datadog.trace_id"
	DatadogRulePSR              = "datadog.rule_psr"
	DatadogProfilingStatus      = "datadog.profiling.status"
	DatadogProfilingErrorReason = "datadog.profiling.error_reason"

	DDSpanId               = "_dd.span_id"
	DDParentSpanId         = "_dd.parent_span_id"
	DDTraceId              = "_dd.trace_id"
	DDRulePSR              = "_dd.rule_psr"
	DDProfilingStatus      = "_dd.profiling.status"
	DDProfilingErrorReason = "_dd.profiling.error_reason"

	// view-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/view-schema.json)
	DatadogDocumentVersion                                  = "datadog.document_version"
	DatadogPageStates                                       = "datadog.page_states"
	DatadogPageStatesState                                  = "datadog.page_states.state"
	DatadogPageStatesStartTime                              = "datadog.page_states.start_time"
	DatadogReplayStatsRecordsCount                          = "datadog.replay_stats.records_count"
	DatadogReplayStatsSegmentsCount                         = "datadog.replay_stats.segments_count"
	DatadogReplayStatsSegmentsTotalRawSize                  = "datadog.replay_stats.segments_total_raw_size"
	DatadogCLSDevicePixelRatio                              = "datadog.cls.device_pixel_ratio"
	DatadogConfigurationStartSessionReplayRecordingManually = "datadog.configuration.start_session_replay_recording_manually"

	DDDocumentVersion                                  = "_dd.document_version"
	DDPageStates                                       = "_dd.page_states"
	DDPageStatesState                                  = "_dd.page_states.state"
	DDPageStatesStartTime                              = "_dd.page_states.start_time"
	DDReplayStatsRecordsCount                          = "_dd.replay_stats.records_count"
	DDReplayStatsSegmentsCount                         = "_dd.replay_stats.segments_count"
	DDReplayStatsSegmentsTotalRawSize                  = "_dd.replay_stats.segments_total_raw_size"
	DDCLSDevicePixelRatio                              = "_dd.cls.device_pixel_ratio"
	DDConfigurationStartSessionReplayRecordingManually = "_dd.configuration.start_session_replay_recording_manually"

	// vital-schema.json (https://github.com/DataDog/rum-events-format/blob/master/schemas/rum/vital-schema.json)
	DatadogVitalComputedValue = "datadog.vital.computed_value"

	DDVitalComputedValue = "_dd.vital.computed_value"
)
