// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

// Package rssshrinker provides functions to reduce the process’ RSS
package rssshrinker

// Setup isn’t implemented on this platform
func Setup() {}
