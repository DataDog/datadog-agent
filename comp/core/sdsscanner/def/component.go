// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sdsscanner exposes shared sensitive-data-scanner (SDS) scanner
// instances as a component, so producers (e.g. a Remote-Config-driven data
// security component) and consumers (e.g. the metrics aggregator) can share
// scanners without depending on each other.
package sdsscanner

// team: sensitive-data-scanner

import (
	sds "github.com/DataDog/datadog-agent/pkg/util/sds"
)

// Component is a registry of named SDS scanners. Producers register scanners;
// consumers look them up and scan. Neither side has to know about the other.
type Component interface {
	// Register builds a scanner from rules and registers it under name. If a
	// scanner is already registered under name, it is closed and replaced. It
	// returns the newly registered scanner and is safe for concurrent use.
	Register(name string, rules []sds.RuleDefinition) (sds.Scanner, error)

	// Unregister removes the scanner registered under name and releases its
	// native resources. It is a no-op if no scanner is registered under name.
	Unregister(name string) error

	// Get returns the scanner registered under name, or false if none exists.
	Get(name string) (sds.Scanner, bool)
}
