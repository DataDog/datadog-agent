// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

// Message is the type of the message handled by a subscription point.  It can
// be any type, but that type must be unique within the codebase.  Do not use
// a basic type like `string` or `int` here.
type Message interface{}
