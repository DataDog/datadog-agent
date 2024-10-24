// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/afero"
)

type notImplementedMethods interface {
	SetFs(afero.Fs)
	IsSet(string) bool
	AllKeys() []string
	SetEnvKeyTransformer(string, func(data string) interface{})
	GetStringSliceE(string) ([]string, error)
	GetStringMapE(string) (map[string]interface{}, error)
	GetStringMapStringE(string) (map[string]string, error)
	GetStringMapStringSliceE(string) (map[string][]string, error)
	GetSizeInBytesE(string) (uint, error)
}

type notImplMethodsImpl struct{}

func (n *notImplMethodsImpl) SetFs(afero.Fs) {
	n.logErrorNotImplemented("SetFs")
}

func (n *notImplMethodsImpl) IsSet(string) bool {
	n.logErrorNotImplemented("IsSet")
	return false
}

func (n *notImplMethodsImpl) AllKeys() []string {
	n.logErrorNotImplemented("AllKeys")
	return nil
}

func (n *notImplMethodsImpl) SetEnvKeyTransformer(string, func(data string) interface{}) {
	n.logErrorNotImplemented("SetEnvKeyTransformer")
}

func (n *notImplMethodsImpl) GetStringSliceE(string) ([]string, error) {
	return nil, n.logErrorNotImplemented("GetStringSliceE")
}

func (n *notImplMethodsImpl) GetStringMapE(string) (map[string]interface{}, error) {
	return nil, n.logErrorNotImplemented("GetStringMapE")
}

func (n *notImplMethodsImpl) GetStringMapStringE(string) (map[string]string, error) {
	return nil, n.logErrorNotImplemented("GetStringMapStringE")
}

func (n *notImplMethodsImpl) GetStringMapStringSliceE(string) (map[string][]string, error) {
	return nil, n.logErrorNotImplemented("GetStringMapStringSliceE")
}

func (n *notImplMethodsImpl) GetSizeInBytesE(string) (uint, error) {
	return 0, n.logErrorNotImplemented("GetSizeInBytesE")
}

func (n *notImplMethodsImpl) logErrorNotImplemented(method string) error {
	err := fmt.Errorf("not implemented: %s", method)
	log.Error(err)
	return err
}
