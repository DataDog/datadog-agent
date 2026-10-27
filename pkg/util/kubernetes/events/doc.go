// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds the part of the Kubernetes events to Datadog events
// mapping that does not depend on a particular source of events: which events
// are worth collecting, which integration they are attributed to, and how their
// title, alert type and aggregation key are derived.
//
// It has no dependency on the tagger, the agent configuration or the API
// server, nor any build tag, so that it can be shared by the
// kubernetes_apiserver check and by components that receive Kubernetes events
// by other means (for example the Datadog exporter).
package events
