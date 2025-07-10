// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var (
	NewMgrKustomizeDirPath = filepath.Join("config")
)

const (
	DefaultMgrImageName = "gcr.io/datadoghq/operator"
	DefaultMgrImgTag    = "latest"
	DefaultMgrFileName  = "e2e-manager.yaml"
	UserData            = `#!/bin/bash
echo "User Data"
echo "Installing kubectl"
snap install kubectl --classic

echo "Verifying kubectl"
kubectl version --client

echo "Installing kubens"
curl -sLo kubens https://github.com/ahmetb/kubectx/releases/download/v0.9.5/kubens
chmod +x kubens
mv kubens /usr/local/bin/

echo '

alias k="kubectl"
alias kg="kubectl get"
alias kgp="kubectl get pod"
alias krm="kubectl delete"
alias krmp="kubectl delete pod"
alias kd="kubectl describe"
alias kdp="kubectl describe pod"
alias ke="kubectl edit"
alias kl="kubectl logs"
alias kx="kubectl exec"
' >> /home/ubuntu/.bashrc
`
)

func loadKustomization(path string) (*types.Kustomization, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var kustomization types.Kustomization
	if err := yaml.Unmarshal(data, &kustomization); err != nil {
		return nil, err
	}

	return &kustomization, nil
}

func saveKustomization(path string, kustomization *types.Kustomization) error {
	data, err := yaml.Marshal(kustomization)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	return nil
}

// updateKustomization Updates kustomization.yaml file in given kustomize directory with extra resources and image name and tag if `IMG` environment variable is set.
func UpdateKustomization(kustomizeDirPath string, kustomizeResourcePaths []string) error {
	var imgName, imgTag string

	kustomizationFilePath := fmt.Sprintf("%s/kustomization.yaml", kustomizeDirPath)
	k, err := loadKustomization(kustomizationFilePath)
	if err != nil {
		return err
	}

	// Update resources with target e2e-manager resource yaml
	if kustomizeResourcePaths != nil {
		// We empty slice to avoid accumulating patches from previous tests
		k.Patches = k.Patches[:0]
		for _, res := range kustomizeResourcePaths {
			k.Patches = append(k.Patches, types.Patch{
				Path: res,
				Target: &types.Selector{
					ResId: resid.NewResIdKindOnly("Deployment", "manager"),
				},
			})
		}
	}

	// Update image
	if os.Getenv("IMG") != "" {
		imgName, imgTag = common.SplitImageString(os.Getenv("IMG"))
	} else {
		imgName = DefaultMgrImageName
		imgTag = DefaultMgrImgTag
	}
	for i, img := range k.Images {
		if img.Name == "controller" {
			k.Images[i].NewName = imgName
			k.Images[i].NewTag = imgTag
		}
	}

	if err := saveKustomization(kustomizationFilePath, k); err != nil {
		return err
	}

	return nil
}
