// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usm provides functionality to detect the most appropriate service name for a process.
package usm

// ServiceMetadata holds information about a service.
// ServiceNameSource is a string enum that represents the source of a generated service name
type ServiceNameSource string

const (
	// CommandLine indicates that the name comes from the command line
	CommandLine ServiceNameSource = "command-line"
	// Laravel indicates that the name comes from the Laravel application name
	Laravel ServiceNameSource = "laravel"
	// Python indicates that the name comes from the Python package name
	Python ServiceNameSource = "python"
	// Nodejs indicates that the name comes from the Node.js package name
	Nodejs ServiceNameSource = "nodejs"
	// Gunicorn indicates that the name comes from the Gunicorn application name
	Gunicorn ServiceNameSource = "gunicorn"
	// Rails indicates that the name comes from the Rails application name
	Rails ServiceNameSource = "rails"
	// Spring indicates that the name comes from the Spring application name
	Spring ServiceNameSource = "spring"
	// JBoss indicates that the name comes from the JBoss application name
	JBoss ServiceNameSource = "jboss"
	// Tomcat indicates that the name comes from the Tomcat application name
	Tomcat ServiceNameSource = "tomcat"
	// WebLogic indicates that the name comes from the WebLogic application name
	WebLogic ServiceNameSource = "weblogic"
	// WebSphere indicates that the name comes from the WebSphere application name
	WebSphere ServiceNameSource = "websphere"
)
