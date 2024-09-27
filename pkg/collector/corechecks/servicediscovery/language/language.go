// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package language provides functionality to detect the programming language for a given process.
package language

// Language represents programming languages.
type Language string

const (
	// Unknown is used when the language could not be detected.
	Unknown Language = "UNKNOWN"
	// Java represents JVM languages.
	Java Language = "jvm"
	// Node represents Node.js.
	Node Language = "nodejs"
	// Python represents Python.
	Python Language = "python"
	// Ruby represents Ruby.
	Ruby Language = "ruby"
	// DotNet represents .Net.
	DotNet Language = "dotnet"
	// Go represents Go.
	Go Language = "go"
	// CPlusPlus represents C++.
	CPlusPlus Language = "cpp"
	// PHP represents PHP.
	PHP Language = "php"
)
