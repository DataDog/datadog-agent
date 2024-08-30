// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	languagedetectionUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	ddkubehelpers "github.com/DataDog/datadog-agent/pkg/util/kubernetes/helpers"
)

type deploymentParser struct {
	annotationsFilter []*regexp.Regexp
	gvr               *schema.GroupVersionResource
}

// NewDeploymentParser initialises and returns a deployment parser
func NewDeploymentParser(annotationsExclude []string) (ObjectParser, error) {
	filters, err := parseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}
	return deploymentParser{
		annotationsFilter: filters,
		gvr: &schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
	}, nil
}

func updateContainerLanguage(cl languagedetectionUtil.ContainersLanguages, container languagedetectionUtil.Container, languages string) {
	if _, found := cl[container]; !found {
		cl[container] = make(languagedetectionUtil.LanguageSet)
	}

	for _, lang := range strings.Split(languages, ",") {
		cl[container][languagedetectionUtil.Language(strings.TrimSpace(lang))] = struct{}{}
	}
}

func (p deploymentParser) Parse(obj interface{}) workloadmeta.Entity {
	deployment := obj.(*appsv1.Deployment)
	containerLanguages := make(languagedetectionUtil.ContainersLanguages)

	for annotation, languages := range deployment.Annotations {

		containerName, isInitContainer := languagedetectionUtil.ExtractContainerFromAnnotationKey(annotation)
		if containerName != "" && languages != "" {

			updateContainerLanguage(
				containerLanguages,
				languagedetectionUtil.Container{
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
			Annotations: filterMapStringKey(deployment.Annotations, p.annotationsFilter),
		},
		Env:                 deployment.Labels[ddkubehelpers.EnvTagLabelKey],
		Service:             deployment.Labels[ddkubehelpers.ServiceTagLabelKey],
		Version:             deployment.Labels[ddkubehelpers.VersionTagLabelKey],
		InjectableLanguages: containerLanguages,
	}
}
