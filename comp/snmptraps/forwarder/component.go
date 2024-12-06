// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder defines a component that receives trap data from the
// listener component, formats it properly, and sends it to the backend.
package forwarder

// team: ndm-core

// Component is the component type.
type Component interface{}
