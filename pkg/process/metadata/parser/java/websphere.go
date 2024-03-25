// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"encoding/xml"
	"path/filepath"

	"github.com/spf13/afero"
)

type (
	// websphereAppDeployment models a deployment.xml descriptor.
	websphereAppDeployment struct {
		XMLName           xml.Name                    `xml:"Deployment"`
		TargetMappings    []websphereTargetMapping    `xml:"deployedObject>targetMappings"`
		DeploymentTargets []websphereDeploymentTarget `xml:"deploymentTargets"`
	}

	// websphereTargetMapping holds information about deployment distributions over targets.
	websphereTargetMapping struct {
		ID           string `xml:"id,attr"`
		Enable       bool   `xml:"enable,attr"`
		ServerTarget string `xml:"target,attr"`
	}

	//websphereDeploymentTarget describes a deployment target.
	websphereDeploymentTarget struct {
		ID         string `xml:"id,attr"`
		ServerName string `xml:"name,attr"`
		NodeName   string `xml:"nodeName,attr"`
	}
)

// isApplicationDeployed checks if an application has been deployed for a target
func isApplicationDeployed(fs afero.Fs, descriptorPath string, nodeName string, serverName string) bool {
	file, err := fs.Open(descriptorPath)
	if err != nil {
		return false
	}
	defer file.Close()
	if !canSafelyParse(file) {
		return false
	}
	var appDeployment websphereAppDeployment
	err = xml.NewDecoder(file).Decode(&appDeployment)
	if err != nil {
		return false
	}
	var matchingTarget string
	for _, target := range appDeployment.DeploymentTargets {
		if target.NodeName == nodeName && target.ServerName == serverName {
			matchingTarget = target.ID
			break
		}
	}
	if len(matchingTarget) == 0 {
		return false
	}
	for _, mapping := range appDeployment.TargetMappings {
		if mapping.Enable && mapping.ServerTarget == matchingTarget {
			return true
		}
	}
	return false
}

// websphereFindDeployedApps finds applications that are enabled in a domainHome for the matched cell, node and server
// If nothing false, it returns false
func websphereFindDeployedApps(domainHome string, args []string, fs afero.Fs) ([]typedDeployment, bool) {
	n := len(args)
	cellName, nodeName, serverName := args[n-3], args[n-2], args[n-1]
	if len(cellName) == 0 || len(nodeName) == 0 || len(serverName) == 0 {
		return nil, false
	}
	matches, err := afero.Glob(fs, filepath.Join(domainHome, "config", "cells", cellName, "applications", "*", "deployments", "*", "deployment.xml"))
	if err != nil {
		return nil, false
	}
	var apps []typedDeployment
	for _, m := range matches {
		if isApplicationDeployed(fs, m, nodeName, serverName) {
			apps = append(apps, typedDeployment{
				path: filepath.Dir(m),
				dt:   ear,
			})
		}
	}
	return apps, len(apps) > 0
}
