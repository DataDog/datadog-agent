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

func (jd javaDetector) detect(args []string) (ServiceMetadata, bool) {
	// Look for dd.service
	if index := slices.IndexFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "-Ddd.service=") }); index != -1 {
		return NewServiceMetadata(strings.TrimPrefix(args[index], "-Ddd.service=")), true
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
				additionalNames = jeeExtractor{ctx: jd.ctx}.extractServiceNamesForJEEServer()
				if strings.HasSuffix(arg, javaJarExtension) {
					// try to see if the application is a spring boot archive and extract its application name
					if len(additionalNames) == 0 {
						if springAppName, ok := NewSpringBootParser(jd.ctx).GetSpringBootAppName(a); ok {
							return NewServiceMetadata(springAppName), true
						}
					}
					return NewServiceMetadata(arg[:len(arg)-len(javaJarExtension)], additionalNames...), true
				}
				if strings.HasPrefix(arg, javaApachePrefix) {
					// take the project name after the package 'org.apache.' while stripping off the remaining package
					// and class name
					arg = arg[len(javaApachePrefix):]
					if idx := strings.Index(arg, "."); idx != -1 {
						return NewServiceMetadata(arg[:idx], additionalNames...), true
					}
				}

				if idx := strings.LastIndex(arg, "."); idx != -1 && idx+1 < len(arg) {
					// take just the class name without the package
					return NewServiceMetadata(arg[idx+1:], additionalNames...), true
				}

				return NewServiceMetadata(arg, additionalNames...), true
			}
		}

		prevArgIsFlag = hasFlagPrefix && !includesAssignment && a != javaJarFlag
	}
	return ServiceMetadata{}, false
}
