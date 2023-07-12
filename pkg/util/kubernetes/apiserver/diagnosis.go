// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	diagnosis.Register("Kubernetes API Server availability", diagnose)
}

// diagnose the API server availability
func diagnose() error {
	c, err := GetAPIClient()
	if err != nil {
		return err
	}
	log.Infof("Detecting OpenShift APIs: %s available", c.DetectOpenShiftAPILevel())

	resourcesNamespace := common.GetResourcesNamespace()
	printRBAC(c, resourcesNamespace)

	myNamespace := common.GetMyNamespace()
	if myNamespace != resourcesNamespace {
		printRBAC(c, myNamespace)
	}

	return nil
}

func printRBAC(client *APIClient, namespace string) {
	rulesReview, err := listRBAC(client, namespace)
	if err != nil {
		log.Errorf("Unable to get RBACs for namespace: %s", namespace)
	}

	log.Infof("RulesReview result for namespace %s:", namespace)
	log.Info(rulesReview.String())
}

func listRBAC(client *APIClient, namespace string) (*authorizationv1.SubjectRulesReviewStatus, error) {
	sar := &authorizationv1.SelfSubjectRulesReview{
		Spec: authorizationv1.SelfSubjectRulesReviewSpec{
			Namespace: namespace,
		},
	}

	response, err := client.Cl.AuthorizationV1().SelfSubjectRulesReviews().Create(context.TODO(), sar, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return &response.Status, nil
}
