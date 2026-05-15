// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package jmxconnection

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName  = "jmx_connection_failure"
	category   = "integration"
	location   = "jmxfetch"
	severity   = "medium"
	source     = "jmxfetch"
	unknownVal = "unknown"
)

// JMXConnectionIssue provides the complete issue template for JMX connection failures
type JMXConnectionIssue struct{}

// NewJMXConnectionIssue creates a new JMX connection failure issue template
func NewJMXConnectionIssue() *JMXConnectionIssue {
	return &JMXConnectionIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for JMX connection failures
func (t *JMXConnectionIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	checkName := context["checkName"]
	if checkName == "" {
		checkName = unknownVal
	}

	host := context["host"]
	if host == "" {
		host = unknownVal
	}

	port := context["port"]
	if port == "" {
		port = unknownVal
	}

	errorMsg := context["error"]
	if errorMsg == "" {
		errorMsg = unknownVal
	}

	description := "The JMX check " + checkName + " failed to connect to " + host + ":" + port +
		". Error: " + errorMsg + ". JMX metrics will not be collected for this integration."

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Verify the JMX endpoint is reachable: nc -zv " + host + " " + port},
		{Order: 2, Text: "Check JMX authentication credentials in conf.yaml"},
		{Order: 3, Text: "Ensure com.sun.management.jmxremote.authenticate=false or correct credentials are set"},
		{Order: 4, Text: "Check firewall rules between agent and JMX port"},
		{Order: 5, Text: "Verify the JVM has JMX remote enabled: -Dcom.sun.management.jmxremote"},
	}

	extra, err := structpb.NewStruct(map[string]any{
		"check_name": checkName,
		"host":       host,
		"port":       port,
		"error":      errorMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		Title:       "JMX Check Cannot Connect to JMX Endpoint",
		Description: description,
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify JMX endpoint reachability and authentication configuration",
			Steps:   steps,
		},
		Tags: []string{"jmx", "integration", "connection"},
	}, nil
}
