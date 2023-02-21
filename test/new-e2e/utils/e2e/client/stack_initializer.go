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
	_, err := getStackInitializers(&env)
	return err
}

func CallStackInitializers[Env any](auth *Authentification, env *Env, upResult auto.UpResult) error {
	initializers, err := getStackInitializers(env)

	for _, initializer := range initializers {
		if err = initializer.setStack(auth, upResult); err != nil {
			return err
		}
	}

	return err
}

func getStackInitializers[Env any](env *Env) ([]stackInitializer, error) {
	var stackInitializers []stackInitializer
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
			return nil, fmt.Errorf("the field %v is not exported", fieldName)
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
		stackInitializers = append(stackInitializers, initializer)
	}
	return stackInitializers, nil
}
