// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

package secrets

// SecretVal defines the structure for secrets in JSON output
type SecretVal struct {
	Value    string `json:"value,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}

// ResolveCallback is the callback type used by the ResolveWithCallback method to send notifications.
//
// ResolveCallback needs to acknowledge the notification by returning 'true' or refuse it and request another notification
// on the parent by returning 'false'.
//
// When exploring a type the walker will emit notifications for each key being updated (when using
// 'ResolveWithCallback'). Some notifier might not be able to handle all notifications in which case it returns 'false'.
// The walker will then retry the notification from the parent key.
//
// This is needed when resolving yaml containing map[string]any. In the case of the datadog configuration the map key
// might contain the character identified as the configuration delimiter.
//
// Example:
// The following agent configuration:
//
//	process_config:
//	  additional_endpoints:
//	    https://app.datadoghq.com:
//	      - some_api_key
//
// Calling 'Set("process_config.additional_endpoints.https://app.datadoghq.com", []string{"some_api_key"})'
// will create the following invalid configuration:
//
//	process_config:
//	  additional_endpoints:
//	    https://app:
//	      datadoghq:
//	        com:
//	          - some_api_key
//
// For this reason the notifier will refuse the notification for
// "process_config.additional_endpoints.https://app.datadoghq.com" but accept the one for the parent
// "process_config.additional_endpoints".
type ResolveCallback func(key []string, value any) bool

// PayloadVersion defines the current payload version sent to a secret backend
const PayloadVersion = "1.0"
