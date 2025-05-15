// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

// Package aws contains database-monitoring specific aurora discovery logic
package aws

// Instance represents an Aurora or RDS instance
type Instance struct {
	Endpoint   string
	Port       int32
	IamEnabled bool
	Engine     string
	DbName     string
	DbmEnabled bool
}
