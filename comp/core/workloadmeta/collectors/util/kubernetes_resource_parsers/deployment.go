// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	ddkube "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type deploymentParser struct {
	annotationsFilter []*regexp.Regexp
}

// NewDeploymentParser initialises and returns a deployment parser
func NewDeploymentParser(annotationsExclude []string) (ObjectParser, error) {
	filters, err := ParseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}
	return deploymentParser{
		annotationsFilter: filters,
	}, nil
}

func updateContainerLanguage(cl languagemodels.ContainersLanguages, container languagemodels.Container, languages string) {
	if _, found := cl[container]; !found {
		cl[container] = make(languagemodels.LanguageSet)
	}

	for _, lang := range strings.Split(languages, ",") {
		cl[container][languagemodels.LanguageName(strings.TrimSpace(lang))] = struct{}{}
	}
}

func (p deploymentParser) Parse(obj interface{}) workloadmeta.Entity {
	// We don't need the full Deployment object. We can extract all we need from the metadata.
	deployment := obj.(*metav1.PartialObjectMetadata)
	containerLanguages := make(languagemodels.ContainersLanguages)

	for annotation, languages := range deployment.Annotations {

		containerName, isInitContainer := languagemodels.ExtractContainerFromAnnotationKey(annotation)
		if containerName != "" && languages != "" {

			updateContainerLanguage(
				containerLanguages,
				languagemodels.Container{
					Name: containerName,
					Init: isInitContainer,
				},
				languages)
		}
	}

	return &workloadmeta.KubernetesDeployment{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesDeployment,
			ID:   deployment.Namespace + "/" + deployment.Name, // we use the namespace/name as id to make it easier for the admission controller to retrieve the corresponding deployment
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        deployment.Name,
			Namespace:   deployment.Namespace,
			Labels:      deployment.Labels,
			Annotations: FilterMapStringKey(deployment.Annotations, p.annotationsFilter),
		},
		Env:                 deployment.Labels[ddkube.EnvTagLabelKey],
		Service:             deployment.Labels[ddkube.ServiceTagLabelKey],
		Version:             deployment.Labels[ddkube.VersionTagLabelKey],
		InjectableLanguages: containerLanguages,
	}
}
