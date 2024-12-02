// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"slices"
	"strings"
)

type javaDetector struct {
	ctx DetectionContext
}

func newJavaDetector(ctx DetectionContext) detector {
	return &javaDetector{ctx: ctx}
}

var vendorToSource = map[serverVendor]ServiceNameSource{
	tomcat:    Tomcat,
	weblogic:  WebLogic,
	websphere: WebSphere,
	jboss:     JBoss,
}

func (jd javaDetector) detect(args []string) (metadata ServiceMetadata, success bool) {
	// Look for dd.service
	if index := slices.IndexFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "-Ddd.service=") }); index != -1 {
		metadata.DDService = strings.TrimPrefix(args[index], "-Ddd.service=")
	}
	prevArgIsFlag := false

	for _, a := range args {
		hasFlagPrefix := strings.HasPrefix(a, "-")
		includesAssignment := strings.ContainsRune(a, '=') ||
			strings.HasPrefix(a, "-X") ||
			strings.HasPrefix(a, "-javaagent:") ||
			strings.HasPrefix(a, "-verbose:")
		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || includesAssignment
		if !shouldSkipArg {
			arg := removeFilePath(a)

			if arg = trimColonRight(arg); isRuneLetterAt(arg, 0) {
				// do JEE detection to see if we can extract additional service names from context roots.
				vendor, additionalNames := jeeExtractor(jd).extractServiceNamesForJEEServer()

				source := CommandLine
				if len(additionalNames) > 0 {
					if vendorSource, ok := vendorToSource[vendor]; ok {
						// The name gets joined to the AdditionalNames, so a part of
						// the name still comes from the command line, but report
						// the source as the web server since that is not easy to
						// guess from looking at the command line.
						source = vendorSource
					}
				}

				if strings.HasSuffix(arg, javaJarExtension) || strings.HasSuffix(arg, javaWarExtension) {
					// try to see if the application is a spring boot archive and extract its application name
					if len(additionalNames) == 0 {
						if springAppName, ok := newSpringBootParser(jd.ctx).GetSpringBootAppName(a); ok {
							success = true
							metadata.SetNames(springAppName, Spring)
							return
						}
					}
					success = true
					metadata.SetNames(arg[:len(arg)-len(javaJarExtension)], source, additionalNames...)
					return
				}
				if strings.HasPrefix(arg, javaApachePrefix) {
					// take the project name after the package 'org.apache.' while stripping off the remaining package
					// and class name
					arg = arg[len(javaApachePrefix):]
					if idx := strings.Index(arg, "."); idx != -1 {
						success = true
						metadata.SetNames(arg[:idx], source, additionalNames...)
						return
					}
				}

				if idx := strings.LastIndex(arg, "."); idx != -1 && idx+1 < len(arg) {
					// take just the class name without the package
					success = true
					metadata.SetNames(arg[idx+1:], source, additionalNames...)
					return
				}

				success = true
				metadata.SetNames(arg, source, additionalNames...)
				return
			}
		}

		prevArgIsFlag = hasFlagPrefix && !includesAssignment && a != javaJarFlag
	}
	return
}
