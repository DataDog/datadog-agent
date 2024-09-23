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

func (jd javaDetector) detect(args []string) (metadata ServiceMetadata, success bool) {
	// Look for dd.service
	if index := slices.IndexFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "-Ddd.service=") }); index != -1 {
		metadata.DDService = strings.TrimPrefix(args[index], "-Ddd.service=")
	}
	prevArgIsFlag := false
	var additionalNames []string

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
				additionalNames = jeeExtractor(jd).extractServiceNamesForJEEServer()
				if strings.HasSuffix(arg, javaJarExtension) {
					// try to see if the application is a spring boot archive and extract its application name
					if len(additionalNames) == 0 {
						if springAppName, ok := newSpringBootParser(jd.ctx).GetSpringBootAppName(a); ok {
							success = true
							metadata.Name = springAppName
							return
						}
					}
					success = true
					metadata.SetNames(arg[:len(arg)-len(javaJarExtension)], additionalNames...)
					return
				}
				if strings.HasPrefix(arg, javaApachePrefix) {
					// take the project name after the package 'org.apache.' while stripping off the remaining package
					// and class name
					arg = arg[len(javaApachePrefix):]
					if idx := strings.Index(arg, "."); idx != -1 {
						success = true
						metadata.SetNames(arg[:idx], additionalNames...)
						return
					}
				}

				if idx := strings.LastIndex(arg, "."); idx != -1 && idx+1 < len(arg) {
					// take just the class name without the package
					success = true
					metadata.SetNames(arg[idx+1:], additionalNames...)
					return
				}

				success = true
				metadata.SetNames(arg, additionalNames...)
				return
			}
		}

		prevArgIsFlag = hasFlagPrefix && !includesAssignment && a != javaJarFlag
	}
	return
}
