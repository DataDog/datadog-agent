// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"reflect"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

type stackInitializer interface {
	setStack(auth *Authentification, stackResult auto.UpResult) error
}

type Authentification struct {
	SSHKey string
}

func CheckEnvStructValid[Env any]() error {
	var env Env
	_, err := getFields(&env)
	return err
}

func CallStackInitializers[Env any](auth *Authentification, env *Env, upResult auto.UpResult) error {
	fields, err := getFields(env)

	for _, field := range fields {
		initializer := field.stackInitializer
		if reflect.TypeOf(initializer).Kind() == reflect.Ptr && reflect.ValueOf(initializer).IsNil() {
			return fmt.Errorf("the field %v of %v is nil", field.name, reflect.TypeOf(env))
		}

		if err = initializer.setStack(auth, upResult); err != nil {
			return err
		}
	}

	return err
}

type field struct {
	stackInitializer stackInitializer
	name             string
}

func getFields[Env any](env *Env) ([]field, error) {
	var fields []field
	envValue := reflect.ValueOf(*env)
	envType := reflect.TypeOf(*env)
	exportedFields := make(map[string]struct{})

	for _, f := range reflect.VisibleFields(envType) {
		if f.IsExported() {
			exportedFields[f.Name] = struct{}{}
		}
	}

	stackInitializerType := reflect.TypeOf((*stackInitializer)(nil)).Elem()
	upResultDeserializerType := reflect.TypeOf((*UpResultDeserializer[any])(nil)).Elem()

	for i := 0; i < envValue.NumField(); i++ {
		fieldName := envValue.Type().Field(i).Name
		if _, found := exportedFields[fieldName]; !found {
			return nil, fmt.Errorf("the field %v in %v is not exported", fieldName, envType)
		}

		initializer, ok := envValue.Field(i).Interface().(stackInitializer)
		if !ok {
			return nil, fmt.Errorf("%v contains %v which doesn't implement %v. See %v",
				envType,
				fieldName,
				stackInitializerType,
				upResultDeserializerType,
			)
		}
		fields = append(fields, field{
			stackInitializer: initializer,
			name:             fieldName,
		})
	}
	return fields, nil
}
