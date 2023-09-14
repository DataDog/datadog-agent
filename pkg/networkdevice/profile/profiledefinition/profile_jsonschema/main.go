// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main holds main related files
package main

import (
	"encoding/json"
	"flag"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/invopop/jsonschema"
	"os"
	"strings"
)

// import (
//
//	"encoding/json"
//	"flag"
//	"github.com/DataDog/datadog-agent/pkg/security/serializers"
//	"github.com/DataDog/datadog-agent/pkg/security/utils"
//	"os"
//	"reflect"
//	"strings"
//	"time"
//
//	"github.com/invopop/jsonschema"
//
// )
func generateBackendJSON(output string) error {
	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
		//Mapper:         jsonTypeMapper,
		//Namer: jsonTypeNamer,
	}

	if err := reflector.AddGoComments("github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition", "./"); err != nil {
		return err
	}
	reflector.CommentMap = cleanupEasyjson(reflector.CommentMap)

	schema := reflector.Reflect(&profiledefinition.DeviceProfileRcConfig{})

	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(output, schemaJSON, 0664)
}

//func jsonTypeMapper(ty reflect.Type) *jsonschema.Schema {
//	if ty == reflect.TypeOf(utils.EasyjsonTime{}) {
//		schema := jsonschema.Reflect(time.Time{})
//		schema.Version = ""
//		return schema
//	}
//	return nil
//}

func cleanupEasyjson(commentMap map[string]string) map[string]string {
	res := make(map[string]string, len(commentMap))
	for name, comment := range commentMap {
		cleaned := strings.TrimSpace(comment)
		cleaned = strings.TrimSuffix(cleaned, "easyjson:json")
		res[name] = strings.TrimSpace(cleaned)
	}
	return res
}

//	func jsonTypeNamer(ty reflect.Type) string {
//		const selinuxPrefix = "selinux"
//
//		base := strings.TrimSuffix(ty.Name(), "Serializer")
//		if strings.HasPrefix(base, selinuxPrefix) {
//			return "SELinux" + strings.TrimPrefix(base, selinuxPrefix)
//		}
//
//		return base
//	}
func main() {
	var (
		output string
	)

	flag.StringVar(&output, "output", "./device_profile_rc_config_schema.json", "Backend JSON schema generated file")
	flag.Parse()

	if err := generateBackendJSON(output); err != nil {
		panic(err)
	}
}
