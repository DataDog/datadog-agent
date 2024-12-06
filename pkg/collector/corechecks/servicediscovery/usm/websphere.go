// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	websphereExtractor struct {
		ctx DetectionContext
	}
)

func newWebsphereExtractor(ctx DetectionContext) vendorExtractor {
	return &websphereExtractor{ctx: ctx}
}

// isApplicationDeployed checks if an application has been deployed for a target
func isApplicationDeployed(fs fs.FS, descriptorPath string, nodeName string, serverName string) (bool, error) {
	file, err := fs.Open(descriptorPath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	reader, err := SizeVerifiedReader(file)
	if err != nil {
		return false, err
	}
	var appDeployment websphereAppDeployment
	err = xml.NewDecoder(reader).Decode(&appDeployment)
	if err != nil {
		return false, err
	}
	var matchingTarget string
	for _, target := range appDeployment.DeploymentTargets {
		if target.NodeName == nodeName && target.ServerName == serverName {
			matchingTarget = target.ID
			break
		}
	}
	if len(matchingTarget) == 0 {
		return false, errors.New("websphere: no matching deployment target found")
	}
	for _, mapping := range appDeployment.TargetMappings {
		if mapping.Enable && mapping.ServerTarget == matchingTarget {
			return true, nil
		}
	}
	return false, nil
}

// findDeployedApps finds applications that are enabled in a domainHome for the matched cell, node and server
// If nothing false, it returns false
func (we websphereExtractor) findDeployedApps(domainHome string) ([]jeeDeployment, bool) {
	n := len(we.ctx.Args)
	if n < 3 {
		return nil, false
	}
	cellName, nodeName, serverName := we.ctx.Args[n-3], we.ctx.Args[n-2], we.ctx.Args[n-1]
	if len(cellName) == 0 || len(nodeName) == 0 || len(serverName) == 0 {
		return nil, false
	}
	matches, err := fs.Glob(we.ctx.fs, path.Join(domainHome, "config", "cells", cellName, "applications", "*", "deployments", "*", "deployment.xml"))
	if err != nil {
		return nil, false
	}
	var apps []jeeDeployment
	for _, m := range matches {
		if ok, err := isApplicationDeployed(we.ctx.fs, m, nodeName, serverName); ok {
			apps = append(apps, jeeDeployment{
				path: path.Dir(m),
				dt:   ear,
			})
		} else if err != nil {
			log.Debugf("websphere: unable to know if an application is deployed (path %q). Err: %v", m, err)
		}
	}
	return apps, len(apps) > 0
}

func (websphereExtractor) customExtractWarContextRoot(_ fs.FS) (string, bool) {
	// websphere deploys everything as an EAR. context root is always expressed in the application.xml
	return "", false
}

func (websphereExtractor) defaultContextRootFromFile(fileName string) (string, bool) {
	return standardExtractContextFromWarName(fileName)
}
