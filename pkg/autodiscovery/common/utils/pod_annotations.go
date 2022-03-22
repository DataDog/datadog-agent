package utils

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

const (
	// KubeAnnotationPrefix is the prefix used by AD in Kubernetes
	// annotations.
	KubeAnnotationPrefix = "ad.datadoghq.com/"

	instancePath   = "instances"
	checkNamePath  = "check_names"
	initConfigPath = "init_configs"
	logsConfigPath = "logs"
	checksPath     = "checks"
	checkIDPath    = "check.id"

	legacyPodAnnotationPrefix = "service-discovery.datadoghq.com/"

	podAnnotationFormat       = KubeAnnotationPrefix + "%s."
	legacyPodAnnotationFormat = legacyPodAnnotationPrefix + "%s."

	checkIDAnnotationFormat = podAnnotationFormat + checkIDPath

	v1PodAnnotationCheckNamesFormat     = podAnnotationFormat + checkNamePath
	v2PodAnnotationChecksFormat         = podAnnotationFormat + checksPath
	legacyPodAnnotationCheckNamesFormat = legacyPodAnnotationFormat + checkNamePath
)

var adPrefixFormats = []string{podAnnotationFormat, legacyPodAnnotationFormat}

// ExtractCheckIDFromPodAnnotations returns whether there is a custom check ID for a given
// container based on the pod annotations
func ExtractCheckIDFromPodAnnotations(annotations map[string]string, containerName string) (string, bool) {
	id, found := annotations[fmt.Sprintf(checkIDAnnotationFormat, containerName)]
	return id, found
}

// ExtractCheckNamesFromPodAnnotations returns check names from a map of pod annotations. In order of
// priority, it prefers annotations v2, v1, and legacy.
func ExtractCheckNamesFromPodAnnotations(annotations map[string]string, adIdentifier string) ([]string, error) {
	// AD annotations v2: "ad.datadoghq.com/redis.checks"
	if checksJSON, found := annotations[fmt.Sprintf(v2PodAnnotationChecksFormat, adIdentifier)]; found {
		checks, err := parseChecksJSON(adIdentifier, checksJSON)
		if err != nil {
			return nil, err
		}

		checkNames := make([]string, 0, len(checks))
		for _, config := range checks {
			checkNames = append(checkNames, config.Name)
		}

		return checkNames, nil
	}

	// AD annotations v1: "ad.datadoghq.com/redis.check_names"
	if checkNamesJSON, found := annotations[fmt.Sprintf(v1PodAnnotationCheckNamesFormat, adIdentifier)]; found {
		checkNames, err := ParseCheckNames(checkNamesJSON)
		if err != nil {
			return nil, fmt.Errorf("cannot parse check names: %w", err)
		}

		return checkNames, nil
	}

	// AD annotations legacy: "service-discovery.datadoghq.com/redis.check_names"
	if checkNamesJSON, found := annotations[fmt.Sprintf(legacyPodAnnotationCheckNamesFormat, adIdentifier)]; found {
		checkNames, err := ParseCheckNames(checkNamesJSON)
		if err != nil {
			return nil, fmt.Errorf("cannot parse check names: %w", err)
		}

		return checkNames, nil
	}

	return nil, nil
}

// ExtractTemplatesFromPodAnnotations looks for autodiscovery configurations in
// a map of annotations and returns them if found. In order of priority, it
// prefers annotations v2, v1, and legacy.
func ExtractTemplatesFromPodAnnotations(entityName string, annotations map[string]string, adIdentifier string) ([]integration.Config, []error) {
	var (
		configs []integration.Config
		errors  []error
		prefix  string
	)

	if checksJSON, found := annotations[fmt.Sprintf(v2PodAnnotationChecksFormat, adIdentifier)]; found {
		// AD annotations v2: "ad.datadoghq.com/redis.checks"
		prefix = fmt.Sprintf(podAnnotationFormat, adIdentifier)
		c, err := parseChecksJSON(entityName, checksJSON)
		if err != nil {
			errors = append(errors, err)
		} else {
			configs = append(configs, c...)
		}
	} else {
		// AD annotations v1: "ad.datadoghq.com/redis.check_names"
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.check_names"
		prefix = findPrefix(annotations, adPrefixFormats, adIdentifier, checkNamePath)
		if prefix != "" {
			c, err := extractCheckTemplatesFromMap(entityName, annotations, prefix)
			if err != nil {
				errors = append(errors, fmt.Errorf("could not extract checks config: %v", err))
			} else {
				configs = append(configs, c...)
			}
		}
	}

	// prefix might not have been detected if there are no check
	// annotations, so we try to find a prefix for log configs
	if prefix == "" {
		// AD annotations v1: "ad.datadoghq.com/redis.logs"
		// AD annotations legacy: "service-discovery.datadoghq.com/redis.logs"
		prefix = findPrefix(annotations, adPrefixFormats, adIdentifier, logsConfigPath)
	}

	if prefix != "" {
		c, err := extractLogsTemplatesFromMap(entityName, annotations, prefix)
		if err != nil {
			errors = append(errors, fmt.Errorf("could not extract logs config: %v", err))
		} else {
			configs = append(configs, c...)
		}
	}

	return configs, errors
}

// parseChecksJSON parses an AD annotation v2
// (ad.datadoghq.com/redis.checks) JSON string into []integration.Config.
func parseChecksJSON(adIdentifier string, checksJSON string) ([]integration.Config, error) {
	var namedChecks map[string]struct {
		Name       string            `json:"name"`
		InitConfig *integration.Data `json:"init_config"`
		Instances  []interface{}     `json:"instances"`
	}

	err := json.Unmarshal([]byte(checksJSON), &namedChecks)
	if err != nil {
		return nil, fmt.Errorf("cannot parse check configuration: %w", err)
	}

	checks := make([]integration.Config, 0, len(namedChecks))
	for name, config := range namedChecks {
		if config.Name != "" {
			name = config.Name
		}

		var initConfig integration.Data
		if config.InitConfig != nil {
			initConfig = *config.InitConfig
		} else {
			initConfig = integration.Data("{}")
		}

		c := integration.Config{
			Name:          name,
			InitConfig:    initConfig,
			ADIdentifiers: []string{adIdentifier},
		}

		for _, i := range config.Instances {
			instance, err := parseJSONObjToData(i)
			if err != nil {
				return nil, err
			}

			c.Instances = append(c.Instances, instance)
		}

		checks = append(checks, c)
	}

	return checks, nil
}

func findPrefix(annotations map[string]string, prefixFmts []string, adIdentifier, suffix string) string {
	for _, prefixFmt := range prefixFmts {
		key := fmt.Sprintf(prefixFmt+suffix, adIdentifier)
		if _, ok := annotations[key]; ok {
			return fmt.Sprintf(prefixFmt, adIdentifier)
		}
	}

	return ""
}
