// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// apmInjectionErrorAnnotationKey this annotation is added when the apm auto-instrumentation admission controller failed to mutate the Pod.
	apmInjectionErrorAnnotationKey = "apm.datadoghq.com/injection-error"
)

type mutatorCore struct {
	config        *Config
	wmeta         workloadmeta.Component
	filter        mutatecommon.MutationFilter
	imageResolver ImageResolver
}

func newMutatorCore(config *Config, wmeta workloadmeta.Component, filter mutatecommon.MutationFilter, imageResolver ImageResolver) *mutatorCore {
	return &mutatorCore{
		config:        config,
		wmeta:         wmeta,
		filter:        filter,
		imageResolver: imageResolver,
	}
}

func (m *mutatorCore) mutatePodContainers(pod *corev1.Pod, cm containerMutator) error {
	return mutatePodContainers(pod, filteredContainerMutator(m.config.containerFilter, cm))
}

func (m *mutatorCore) injectTracers(pod *corev1.Pod, config extractedPodLibInfo) error {
	if len(config.libs) == 0 {
		return nil
	}

	requirements, injectionDecision := initContainerResourceRequirements(pod, m.config.defaultResourceRequirements)
	if injectionDecision.skipInjection {
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[apmInjectionErrorAnnotationKey] = injectionDecision.message
		return nil
	}

	var (
		lastError      error
		startTime      = time.Now()
		configInjector = &libConfigInjector{}
		injectionType  = config.source.injectionType()
		autoDetected   = config.source.isFromLanguageDetection()

		ustEnvVarMutator = m.ustEnvVarMutator(pod)

		// initContainerMutators are resource and security constraints
		// to all the init containers the init containers that we create.
		initContainerMutators = append(
			m.newInitContainerMutators(requirements, pod.Namespace),
			ustEnvVarMutator,
		)
		injectorOptions = libRequirementOptions{
			containerFilter:       m.config.containerFilter,
			initContainerMutators: initContainerMutators,
		}

		injector          = m.newInjector(pod, startTime, injectorOptions)
		containerMutators = containerMutators{
			config.languageDetection.containerMutator(),
			ustEnvVarMutator,
		}
	)

	// Inject env variables used for Onboarding KPIs propagation...
	// if Single Step Instrumentation is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_single_step
	// if local library injection is enabled, inject DD_INSTRUMENTATION_INSTALL_TYPE:k8s_lib_injection
	if err := m.mutatePodContainers(pod, config.source.containerMutator()); err != nil {
		return err
	}

	if err := injector.podMutator().mutatePod(pod); err != nil {
		// setting the language tag to `injector` because this injection is not related to a specific supported language
		metrics.LibInjectionErrors.Inc("injector", strconv.FormatBool(autoDetected), injectionType)
		lastError = err
		log.Errorf("Cannot inject library injector into pod %s: %s", mutatecommon.PodString(pod), err)
	}

	for _, lib := range config.libs {
		injected := false
		langStr := string(lib.lang)
		defer func() {
			metrics.LibInjectionAttempts.Inc(langStr, strconv.FormatBool(injected), strconv.FormatBool(autoDetected), injectionType)
		}()

		if err := lib.podMutator(libRequirementOptions{
			containerFilter:       m.config.containerFilter,
			containerMutators:     containerMutators,
			initContainerMutators: initContainerMutators,
			podMutators:           []podMutator{configInjector.podMutator(lib.lang)},
		}, m.imageResolver).mutatePod(pod); err != nil {
			metrics.LibInjectionErrors.Inc(langStr, strconv.FormatBool(autoDetected), injectionType)
			lastError = err
			continue
		}

		injected = true
	}

	if err := configInjector.podMutator(language("all")).mutatePod(pod); err != nil {
		metrics.LibInjectionErrors.Inc("all", strconv.FormatBool(autoDetected), injectionType)
		lastError = err
		log.Errorf("Cannot inject library configuration into pod %s: %s", mutatecommon.PodString(pod), err)
	}

	if m.filter.IsNamespaceEligible(pod.Namespace) {
		_ = basicLibConfigInjector{}.mutatePod(pod)
	}

	return lastError
}

// serviceNameMutator will attempt to find a service name to
// inject into the pods containers if SSI is enabled.
//
// This is kind of gross, and would ideally not happen more than in
// one place but we made a decision to infer DD_SERVICE in the auto-instrumentation
// webhook a while ago and customers might be relying on this behavior.
//
// We have another webhook that does something really similar: tagsFromLabels and
// it this is where the responsibility should generally.
//
// The big difference between the two is that tagsFromLabels looks at the label
// metadata and we might override it and this one will look for the _name_ of the
// owner resource.
//
// The intention is to have this always run last so that we fallback to the owner
// name in cases of missing labels coming from the resource or its owner.
//
// We want to get rid of the behavior when we are triggering the fallback _and_
// it applies: https://datadoghq.atlassian.net/browse/INPLAT-458
func (m *mutatorCore) serviceNameMutator(pod *corev1.Pod) containerMutator {
	return newServiceNameMutator(pod, m.config.podMetaAsTags)
}

// ustEnvVarMutator will attempt to find a ust env var to inject into the pods containers if SSI is enabled.
//
// This is used to inject the version and env tags into the pods containers.
//
// The service tag/name is handled separately in the serviceNameMutator for legacy reasons.
func (m *mutatorCore) ustEnvVarMutator(pod *corev1.Pod) containerMutator {
	var mutators containerMutators
	if !m.filter.IsNamespaceEligible(pod.Namespace) {
		return mutators
	}

	for tag, envVarName := range map[string]string{
		tags.Version: kubernetes.VersionTagEnvVar,
		tags.Env:     kubernetes.EnvTagEnvVar,
	} {
		if mutator := ustEnvVarMutatorForPodMeta(pod, m.config.podMetaAsTags, tag, envVarName); mutator != nil {
			mutators = append(mutators, mutator)
		}
	}

	if mutator := m.serviceNameMutator(pod); mutator != nil {
		mutators = append(mutators, mutator)
	}

	return mutators
}

// newInitContainerMutators constructs container mutators for behavior
// that is common and passed to the init containers we create.
//
// At this point in time it is: resource requirements and security contexts.
func (m *mutatorCore) newInitContainerMutators(
	requirements corev1.ResourceRequirements,
	nsName string,
) containerMutators {
	securityContext := m.config.initSecurityContext
	if securityContext == nil {
		nsLabels, err := getNamespaceLabels(m.wmeta, nsName)
		if err != nil {
			log.Warnf("error getting labels for namespace=%s: %s", nsName, err)
		} else if val, ok := nsLabels["pod-security.kubernetes.io/enforce"]; ok && val == "restricted" {
			// https://datadoghq.atlassian.net/browse/INPLAT-492
			securityContext = defaultRestrictedSecurityContext
		}
	}

	mutators := []containerMutator{
		containerResourceRequirements{requirements},
	}

	if securityContext != nil {
		mutators = append(mutators, containerSecurityContext{securityContext})
	}

	return mutators
}

// newInjector creates an injector instance for this pod.
func (m *mutatorCore) newInjector(pod *corev1.Pod, startTime time.Time, lopts libRequirementOptions) *injector {
	opts := []injectorOption{
		injectorWithLibRequirementOptions(lopts),
		injectorWithImageTag(m.config.Instrumentation.InjectorImageTag, m.imageResolver),
	}

	for _, e := range []annotationExtractor[injectorOption]{
		injectorVersionAnnotationExtractorFunc(m.imageResolver),
		injectorImageAnnotationExtractor,
		injectorDebugAnnotationExtractor,
	} {
		opt, err := e.extract(pod)
		if err != nil {
			if !isErrAnnotationNotFound(err) {
				log.Warnf("error extracting injector annotation %s in single step", e.key)
			}
			continue
		}
		opts = append(opts, opt)
	}

	return newInjector(startTime, m.config.containerRegistry, opts...)
}

func extractLibrariesFromAnnotations(pod *corev1.Pod, containerRegistry string) []libInfo {
	var (
		libList        []libInfo
		extractLibInfo = func(e annotationExtractor[libInfo]) {
			i, err := e.extract(pod)
			if err != nil {
				if !isErrAnnotationNotFound(err) {
					log.Warnf("error extracting annotation for key %s", e.key)
				}
			} else {
				libList = append(libList, i)
			}
		}
	)
	for _, l := range supportedLanguages {
		extractLibInfo(l.customLibAnnotationExtractor())
		extractLibInfo(l.libVersionAnnotationExtractor(containerRegistry))
		for _, ctr := range pod.Spec.Containers {
			extractLibInfo(l.ctrCustomLibAnnotationExtractor(ctr.Name))
			extractLibInfo(l.ctrLibVersionAnnotationExtractor(ctr.Name, containerRegistry))
		}
	}

	return libList
}

func (m *mutatorCore) initExtractedLibInfo(pod *corev1.Pod) extractedPodLibInfo {
	// it's possible to get here without single step being enabled, and the pod having
	// annotations on it to opt it into pod mutation, we disambiguate those two cases.
	var (
		source            = libInfoSourceLibInjection
		languageDetection *libInfoLanguageDetection
	)

	if m.filter.IsNamespaceEligible(pod.Namespace) {
		source = libInfoSourceSingleStepInstrumentation
		languageDetection = m.getLibrariesLanguageDetection(pod)
	}

	return extractedPodLibInfo{
		source:            source,
		languageDetection: languageDetection,
	}
}

// getLibrariesLanguageDetection returns the languages that were detected by process language detection.
//
// The languages information is available in workloadmeta-store
// and attached on the pod's owner.
func (m *mutatorCore) getLibrariesLanguageDetection(pod *corev1.Pod) *libInfoLanguageDetection {
	if !m.config.LanguageDetection.Enabled ||
		!m.config.LanguageDetection.ReportingEnabled {
		return nil
	}

	return &libInfoLanguageDetection{
		libs:             m.getAutoDetectedLibraries(pod),
		injectionEnabled: m.config.LanguageDetection.InjectDetected,
	}
}

// getAutoDetectedLibraries constructs the libraries to be injected if the languages
// were stored in workloadmeta store based on owner annotations
// (for example: Deployment, DaemonSet, StatefulSet).
func (m *mutatorCore) getAutoDetectedLibraries(pod *corev1.Pod) []libInfo {
	ownerName, ownerKind, found := getOwnerNameAndKind(pod)
	if !found {
		return nil
	}

	store := m.wmeta
	if store == nil {
		return nil
	}

	// Currently we only support deployments
	switch ownerKind {
	case "Deployment":
		return getLibListFromDeploymentAnnotations(store, ownerName, pod.Namespace, m.config.containerRegistry)
	default:
		log.Debugf("This ownerKind:%s is not yet supported by the process language auto-detection feature", ownerKind)
		return nil
	}
}

// The config for the security products has three states: <unset> | true | false.
// This is because the products themselves have treat these cases differently:
// * <unset> - product disactivated but can be activated remotely
// * true - product activated, not overridable remotely
// * false - product disactivated, not overridable remotely
func securityClientLibraryConfigMutators(datadogConfig config.Component) containerMutators {
	asmEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm.enabled")
	iastEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.iast.enabled")
	asmScaEnabled := getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm_sca.enabled")

	var mutators []containerMutator
	if asmEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromBoolMutator("DD_APPSEC_ENABLED", asmEnabled))
	}

	if iastEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromBoolMutator("DD_IAST_ENABLED", iastEnabled))
	}

	if asmScaEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromBoolMutator("DD_APPSEC_SCA_ENABLED", asmScaEnabled))
	}

	return mutators
}

// The config for profiling has four states: <unset> | "auto" | "true" | "false".
// * <unset> - profiling not activated, but can be activated remotely
// * "true" - profiling activated unconditionally, not overridable remotely
// * "false" - profiling deactivated, not overridable remotely
// * "auto" - profiling activates per-process heuristically, not overridable remotely
func profilingClientLibraryConfigMutators(datadogConfig config.Component) containerMutators {
	profilingEnabled := getOptionalStringValue(datadogConfig, "admission_controller.auto_instrumentation.profiling.enabled")

	var mutators []containerMutator
	if profilingEnabled != nil {
		mutators = append(mutators, newConfigEnvVarFromStringMutator("DD_PROFILING_ENABLED", profilingEnabled))
	}

	return mutators
}

func getNamespaceLabels(wmeta workloadmeta.Component, name string) (map[string]string, error) {
	id := util.GenerateKubeMetadataEntityID("", "namespaces", "", name)
	ns, err := wmeta.GetKubernetesMetadata(id)
	if err != nil {
		return nil, fmt.Errorf("error getting namespace metadata for ns=%s: %w", name, err)
	}

	return ns.EntityMeta.Labels, nil
}

type libInfoLanguageDetection struct {
	libs             []libInfo
	injectionEnabled bool
}

func (l *libInfoLanguageDetection) containerMutator() containerMutator {
	return containerMutatorFunc(func(c *corev1.Container) error {
		if l == nil {
			return nil
		}

		var langs []string
		for _, lib := range l.libs {
			if lib.ctrName == c.Name { // strict container name matching
				langs = append(langs, string(lib.lang))
			}
		}

		// N.B.
		// We report on the languages detected regardless
		// of if it is empty or not to disambiguate the empty state
		// language_detection reporting being disabled.
		if err := (containerMutators{
			envVar{
				key:     "DD_INSTRUMENTATION_LANGUAGES_DETECTED",
				valFunc: identityValFunc(strings.Join(langs, ",")),
			},
			envVar{
				key:     "DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED",
				valFunc: identityValFunc(strconv.FormatBool(l.injectionEnabled)),
			},
		}).mutateContainer(c); err != nil {
			return err
		}

		return nil
	})
}

// libInfoSource describes where we got the libraries from for
// injection and is used to set up metrics/telemetry. See
// Webhook.injectAutoInstruConfig for usage.
type libInfoSource int

const (
	// libInfoSourceLibInjection is when the user sets up annotations on their pods for
	// library injection and single step is disabled.
	libInfoSourceLibInjection libInfoSource = iota
	// libInfoSourceSingleStepInstrumentation is when we are using the instrumentation config
	// to determine which libraries to inject.
	libInfoSourceSingleStepInstrumentation
	// libInfoSourceSingleStepLanguageDetection is when we use the language detection
	// annotation to determine which libs to inject.
	libInfoSourceSingleStepLangaugeDetection
)

// injectionType produces a string to distinguish between if
// we're using "single step" or "lib injection" for metrics and logging.
func (s libInfoSource) injectionType() string {
	switch s {
	case libInfoSourceSingleStepInstrumentation, libInfoSourceSingleStepLangaugeDetection:
		return singleStepInstrumentationInstallType
	case libInfoSourceLibInjection:
		return localLibraryInstrumentationInstallType
	default:
		return "unknown"
	}
}

// isFromLanguageDetection tells us whether this source comes from
// the language detection reporting and annotation.
func (s libInfoSource) isFromLanguageDetection() bool {
	return s == libInfoSourceSingleStepLangaugeDetection
}

func (s libInfoSource) instrumentationInstallTime() string {
	instrumentationInstallTime := os.Getenv(instrumentationInstallTimeEnvVarName)
	if instrumentationInstallTime == "" {
		instrumentationInstallTime = common.ClusterAgentStartTime
	}

	return instrumentationInstallTime
}

// containerMutator creates a containerMutator for
// telemetry environment variables pertaining to:
//
// - installation_time
// - install_id
// - injection_type
func (s libInfoSource) containerMutator() containerMutator {
	return containerMutators{
		// inject DD_INSTRUMENTATION_INSTALL_TIME with current Unix time
		envVarMutator(corev1.EnvVar{
			Name:  instrumentationInstallTimeEnvVarName,
			Value: s.instrumentationInstallTime(),
		}),
		// inject DD_INSTRUMENTATION_INSTALL_ID with UUID created during the Agent install time
		envVarMutator(corev1.EnvVar{
			Name:  instrumentationInstallIDEnvVarName,
			Value: os.Getenv(instrumentationInstallIDEnvVarName),
		}),
		envVarMutator(corev1.EnvVar{
			Name:  instrumentationInstallTypeEnvVarName,
			Value: s.injectionType(),
		}),
	}
}

type extractedPodLibInfo struct {
	// libs are the libraries we are going to attempt to inject into the given pod.
	libs []libInfo
	// languageDetection is set when we ran/used the language-detection annotation.
	languageDetection *libInfoLanguageDetection
	// source is where we got the data from, used for telemetry later.
	source libInfoSource
}

func (e extractedPodLibInfo) withLibs(l []libInfo) extractedPodLibInfo {
	e.libs = l
	return e
}

func (e extractedPodLibInfo) useLanguageDetectionLibs() (extractedPodLibInfo, bool) {
	if e.languageDetection != nil && len(e.languageDetection.libs) > 0 && e.languageDetection.injectionEnabled {
		e.libs = e.languageDetection.libs
		e.source = libInfoSourceSingleStepLangaugeDetection
		return e, true
	}

	return e, false
}

// podSumRessourceRequirements computes the sum of cpu/memory necessary for the whole pod.
// This is computed as max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
// for both limit and request
// https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/#resource-sharing-within-containers
func podSumRessourceRequirements(pod *corev1.Pod) corev1.ResourceRequirements {
	ressourceRequirement := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	for _, k := range [2]corev1.ResourceName{corev1.ResourceMemory, corev1.ResourceCPU} {
		// Take max(initContainer ressource)
		maxInitContainerLimit := resource.Quantity{}
		maxInitContainerRequest := resource.Quantity{}
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if initContainerIsSidecar(c) {
				// This is a sidecar container, since it will run alongside the main containers
				// we need to add it's resources to the main container's resources
				continue
			}
			if limit, ok := c.Resources.Limits[k]; ok {
				if limit.Cmp(maxInitContainerLimit) == 1 {
					maxInitContainerLimit = limit
				}
			}
			if request, ok := c.Resources.Requests[k]; ok {
				if request.Cmp(maxInitContainerRequest) == 1 {
					maxInitContainerRequest = request
				}
			}
		}

		// Take sum(container resources) + sum(sidecar containers)
		limitSum := resource.Quantity{}
		reqSum := resource.Quantity{}
		for i := range pod.Spec.Containers {
			c := &pod.Spec.Containers[i]
			if l, ok := c.Resources.Limits[k]; ok {
				limitSum.Add(l)
			}
			if l, ok := c.Resources.Requests[k]; ok {
				reqSum.Add(l)
			}
		}
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if !initContainerIsSidecar(c) {
				continue
			}
			if l, ok := c.Resources.Limits[k]; ok {
				limitSum.Add(l)
			}
			if l, ok := c.Resources.Requests[k]; ok {
				reqSum.Add(l)
			}
		}

		// Take max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
		if limitSum.Cmp(maxInitContainerLimit) == 1 {
			maxInitContainerLimit = limitSum
		}
		if reqSum.Cmp(maxInitContainerRequest) == 1 {
			maxInitContainerRequest = reqSum
		}

		// Ensure that the limit is greater or equal to the request
		if maxInitContainerRequest.Cmp(maxInitContainerLimit) == 1 {
			maxInitContainerLimit = maxInitContainerRequest
		}

		if maxInitContainerLimit.CmpInt64(0) == 1 {
			ressourceRequirement.Limits[k] = maxInitContainerLimit
		}
		if maxInitContainerRequest.CmpInt64(0) == 1 {
			ressourceRequirement.Requests[k] = maxInitContainerRequest
		}
	}

	return ressourceRequirement
}

type injectionResourceRequirementsDecision struct {
	skipInjection bool
	message       string
}

// initContainerResourceRequirements computes init container cpu/memory requests and limits.
// There are two cases:
//
//  1. If a resource quantity was set in config, we use it
//
//  2. If no quantity was set, we try to use as much of the resource as we can without impacting
//     pod scheduling.
//     Init containers are run one after another. This means that any single init container can use
//     the maximum amount of the resource requested by the original pod wihtout changing how much of
//     this resource is necessary.
//     In particular, for the QoS Guaranteed Limits and Requests have to be equal for every container.
//     which means that the max amount of request/limits that we compute is going to be equal to each other
//     so our init container will also have request == limit.
//
//     In the 2nd case, of we wouldn't have enough memory, we bail on injection
func initContainerResourceRequirements(pod *corev1.Pod, conf initResourceRequirementConfiguration) (requirements corev1.ResourceRequirements, decision injectionResourceRequirementsDecision) {
	requirements = corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}
	podRequirements := podSumRessourceRequirements(pod)
	insufficientResourcesMessage := "The overall pod's containers limit is too low"
	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		if q, ok := conf[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
		} else {
			if maxPodLim, ok := podRequirements.Limits[k]; ok {
				// If the pod before adding instrumentation init containers would have had a limits smaller than
				// a certain amount, we just don't do anything, for two reasons:
				// 1. The init containers need quite a lot of memory/CPU in order to not OOM or initialize in reasonnable time
				// 2. The APM libraries themselves will increase footprint of the container by a
				//   non trivial amount, and we don't want to cause issues for constrained apps
				switch k {
				case corev1.ResourceMemory:
					if minimumMemoryLimit.Cmp(maxPodLim) == 1 {
						decision.skipInjection = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), minimumMemoryLimit.String())
					}
				case corev1.ResourceCPU:
					if minimumCPULimit.Cmp(maxPodLim) == 1 {
						decision.skipInjection = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), minimumCPULimit.String())
					}
				default:
					// We don't support other resources
				}
				requirements.Limits[k] = maxPodLim
			}
			if maxPodReq, ok := podRequirements.Requests[k]; ok {
				requirements.Requests[k] = maxPodReq
			}
		}
	}
	if decision.skipInjection {
		log.Debug(insufficientResourcesMessage)
		decision.message = insufficientResourcesMessage
	}
	return requirements, decision
}

func containsInitContainer(pod *corev1.Pod, initContainerName string) bool {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == initContainerName {
			return true
		}
	}

	return false
}

func initContainerIsSidecar(container *corev1.Container) bool {
	return container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways
}
