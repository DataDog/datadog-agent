// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/invopop/jsonschema"
)

func usage() {
	fmt.Fprintf(os.Stderr, "k8s_schema_generator <kubernetes_worker_node|kubernetes_master_node>\n")
	os.Exit(1)
}

// This tool produces a JSON schema of the k8sconfig types hierarchy.
// go run ./pkg/compliance/tools/k8s_schema_generator/main.go
func main() {
	if len(os.Args) < 2 {
		usage()
	}
	reflector := &jsonschema.Reflector{
		RequiredFromJSONSchemaTags: true,
	}
	schema := reflector.Reflect(&k8sconfig.K8sNodeConfig{})
	resource := os.Args[1]
	switch resource {
	case "kubernetes_worker_node":
		n, ok := schema.Definitions["K8sNodeConfig"]
		if !ok {
			log.Fatal("bad schema: missing K8sNodeConfig definition")
		}
		cs, ok := n.Properties.Get("components")
		if !ok {
			log.Fatal("bad schema: missing components properties")
		}
		n.Properties.Delete("manifests")
		c := cs.(*jsonschema.Schema)
		c.Properties.Delete("kubeControllerManager")
		c.Properties.Delete("kubeApiserver")
		c.Properties.Delete("kubeScheduler")
		c.Properties.Delete("kubeControllerManager")
		c.Properties.Delete("etcd")
		delete(schema.Definitions, "K8sKubeApiserverConfig")
		delete(schema.Definitions, "K8sEtcdConfig")
		delete(schema.Definitions, "K8sKubeControllerManagerConfig")
		delete(schema.Definitions, "K8sKubeSchedulerConfig")
	case "kubernetes_master_node":
		// do nothing
	default:
		log.Fatalf(`resource should be "kubernetes_master_node" or "kubernetes_worker_node": was %q`, resource)
	}
	b, err := json.MarshalIndent(schema, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
