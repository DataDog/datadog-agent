// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subscriptions provides support for managing subscriptions between
// components.
//
// This package provides a simple subscription implementation with its
// Transmitter and Receiver types.  Send messages with tx.Notify() and receive
// them with <-rx.Chan().
//
// Subscriptions are keyed by the message type.  Messages can be of any type, but
// that type must be unique within the agent codebase.
//
// See the components documentation (doc/components) for more detailed usage
// information.
//
// # Warning
//
// This package is not intended for high-bandwidth messaging such as metric
// samples.  It use should be limited to events that occur on a per-second
// scale.
package subscriptions
