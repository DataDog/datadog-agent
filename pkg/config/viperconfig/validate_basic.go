// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package viperconfig

import (
	"reflect"
	"runtime"
	"strings"
)

// TODO: Callers that are using SetWithoutSource improperly, need to be fixed
var allowlistCaller = []string{
	"comp/api/api/apiimpl/internal/config/endpoint_test.go",
	"comp/autoscaling/datadogclient/impl/client_test.go",
	"comp/autoscaling/datadogclient/impl/status_test.go",
	"comp/core/autodiscovery/listeners/dbm_aurora_test.go",
	"comp/core/autodiscovery/listeners/dbm_rds_test.go",
	"comp/core/autodiscovery/listeners/snmp_test.go",
	"comp/core/ipc/impl/ipc_test.go",
	"comp/core/profiler/impl/profiler_test.go",
	"comp/core/workloadfilter/catalog/filter_config_test.go",
	"comp/core/workloadmeta/collectors/internal/kubeapiserver/kubeapiserver_test.go",
	"comp/logs/agent/config/config_keys_test.go",
	"comp/logs/agent/config/config_test.go",
	"comp/logs/agent/config/endpoints_test.go",
	"comp/metadata/host/hostimpl/host_test.go",
	"comp/metadata/resources/resourcesimpl/resources_test.go",
	"comp/networkpath/npcollector/npcollectorimpl/config_test.go",
	"comp/networkpath/npcollector/npcollectorimpl/npcollector_testutils.go",
	"comp/process/agent/agent_linux_test.go",
	"comp/snmptraps/config/config_test.go",
	"pkg/collector/corechecks/snmp/status/status_test.go",
	"pkg/fleet/installer/packages/embedded/tmpl/main_test.go",
}

// ValidateBasicTypes returns true if the argument is made of only basic types
func ValidateBasicTypes(value interface{}) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	if validate(v) {
		return true
	}

	// Allow existing callers that are using SetWithoutSource. Fix these later
	for _, stackSkip := range []int{2, 3} {
		_, absfile, _, _ := runtime.Caller(stackSkip)
		for _, allowSource := range allowlistCaller {
			if strings.HasSuffix(absfile, allowSource) {
				return true
			}
		}
	}

	return false
}

func validate(v reflect.Value) bool {
	if v.Interface() == nil {
		return true
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	case reflect.Struct:
		return false
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !validate(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			if !validate(key) {
				return false
			}
			if !validate(iter.Value()) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
