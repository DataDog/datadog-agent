// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"context"
	"encoding/json"
	"sync"
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import (
	"C"
)

// GetVersion exposes the version of the agent to Python checks.
//
//export GetVersion
func GetVersion(agentVersion **C.char) {
	av, _ := version.Agent()
	// version will be free by rtloader when it's done with it
	*agentVersion = TrackedCString(av.GetNumber())
}

// GetHostname exposes the current hostname of the agent to Python checks.
//
//export GetHostname
func GetHostname(hostname **C.char) {
	goHostname, err := hostnameUtil.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err)
		goHostname = ""
	}
	// hostname will be free by rtloader when it's done with it
	*hostname = TrackedCString(goHostname)
}

// GetClusterName exposes the current clustername (if it exists) of the agent to Python checks.
//
//export GetClusterName
func GetClusterName(clusterName **C.char) {
	goHostname, _ := hostnameUtil.Get(context.TODO())
	goClusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), goHostname)
	// clusterName will be free by rtloader when it's done with it
	*clusterName = TrackedCString(goClusterName)
}

// TracemallocEnabled exposes the tracemalloc configuration of the agent to Python checks.
//
//export TracemallocEnabled
func TracemallocEnabled() C.bool {
	return C.bool(config.Datadog.GetBool("tracemalloc_debug"))
}

// Headers returns a basic set of HTTP headers that can be used by clients in Python checks.
//
//export Headers
func Headers(yamlPayload **C.char) {
	h := util.HTTPHeaders()

	data, err := yaml.Marshal(h)
	if err != nil {
		log.Errorf("datadog_agent: could not Marshal headers: %s", err)
		*yamlPayload = nil
		return
	}
	// yamlPayload will be free by rtloader when it's done with it
	*yamlPayload = TrackedCString(string(data))
}

// GetConfig returns a value from the agent configuration.
// Indirectly used by the C function `get_config` that's mapped to `datadog_agent.get_config`.
//
//export GetConfig
func GetConfig(key *C.char, yamlPayload **C.char) {
	goKey := C.GoString(key)
	if !config.Datadog.IsSet(goKey) {
		*yamlPayload = nil
		return
	}

	value := config.Datadog.Get(goKey)
	data, err := yaml.Marshal(value)
	if err != nil {
		log.Errorf("could not convert configuration value '%v' to YAML: %s", value, err)
		*yamlPayload = nil
		return
	}
	// yaml Payload will be free by rtloader when it's done with it
	*yamlPayload = TrackedCString(string(data))
}

// LogMessage logs a message from python through the agent logger (see
// https://docs.python.org/2.7/library/logging.html#logging-levels)
//
//export LogMessage
func LogMessage(message *C.char, logLevel C.int) {
	goMsg := C.GoString(message)

	switch logLevel {
	case 50: // CRITICAL
		log.Critical(goMsg)
	case 40: // ERROR
		log.Error(goMsg)
	case 30: // WARNING
		log.Warn(goMsg)
	case 20: // INFO
		log.Info(goMsg)
	case 10: // DEBUG
		log.Debug(goMsg)
	// Custom log level defined in:
	// https://github.com/DataDog/integrations-core/blob/master/datadog_checks_base/datadog_checks/base/log.py
	case 7: // TRACE
		log.Trace(goMsg)
	default: // unknown log level
		log.Info(goMsg)
	}

	return
}

// SetExternalTags adds a set of tags for a given hostname to the External Host
// Tags metadata provider cache.
//
//export SetExternalTags
func SetExternalTags(hostname *C.char, sourceType *C.char, tags **C.char) {
	hname := C.GoString(hostname)
	stype := C.GoString(sourceType)
	tagsStrings := []string{}

	pStart := unsafe.Pointer(tags)
	size := unsafe.Sizeof(*tags)
	for i := 0; ; i++ {
		pTag := *(**C.char)(unsafe.Pointer(uintptr(pStart) + size*uintptr(i)))
		if pTag == nil {
			break
		}
		tag := C.GoString(pTag)
		tagsStrings = append(tagsStrings, tag)
	}

	externalhost.SetExternalTags(hname, stype, tagsStrings)
}

// SetCheckMetadata updates a metadata value for one check instance in the cache.
// Indirectly used by the C function `set_check_metadata` that's mapped to `datadog_agent.set_check_metadata`.
//
//export SetCheckMetadata
func SetCheckMetadata(checkID, name, value *C.char) {
	cid := C.GoString(checkID)
	key := C.GoString(name)
	val := C.GoString(value)

	inventories.SetCheckMetadata(cid, key, val)
}

// WritePersistentCache stores a value for one check instance
// Indirectly used by the C function `write_persistent_cache` that's mapped to `datadog_agent.write_persistent_cache`.
//
//export WritePersistentCache
func WritePersistentCache(key, value *C.char) {
	keyName := C.GoString(key)
	val := C.GoString(value)
	persistentcache.Write(keyName, val) //nolint:errcheck
}

// ReadPersistentCache retrieves a value for one check instance
// Indirectly used by the C function `read_persistent_cache` that's mapped to `datadog_agent.read_persistent_cache`.
//
//export ReadPersistentCache
func ReadPersistentCache(key *C.char) *C.char {
	keyName := C.GoString(key)
	data, err := persistentcache.Read(keyName)
	if err != nil {
		log.Errorf("Failed to read cache %s: %s", keyName, err)
		return nil
	}
	return TrackedCString(data)
}

var (
	// one obfuscator instance is shared across all python checks. It is not threadsafe but that is ok because
	// the GIL is always locked when calling c code from python which means that the exported functions in this file
	// will only ever be called by one goroutine at a time
	obfuscator       *obfuscate.Obfuscator
	obfuscatorLoader sync.Once
)

// lazyInitObfuscator initializes the obfuscator the first time it is used. We can't initialize during the package init
// because the obfuscator depends on config.Datadog and it isn't guaranteed to be initialized during package init, but
// will definitely be initialized by the time one of the python checks runs
func lazyInitObfuscator() *obfuscate.Obfuscator {
	obfuscatorLoader.Do(func() {
		var cfg obfuscate.Config
		if err := config.Datadog.UnmarshalKey("apm_config.obfuscation", &cfg); err != nil {
			log.Errorf("Failed to unmarshal apm_config.obfuscation: %s", err.Error())
			cfg = obfuscate.Config{}
		}
		if !cfg.SQLExecPlan.Enabled {
			cfg.SQLExecPlan = defaultSQLPlanObfuscateSettings
		}
		if !cfg.SQLExecPlanNormalize.Enabled {
			cfg.SQLExecPlanNormalize = defaultSQLPlanNormalizeSettings
		}
		obfuscator = obfuscate.NewObfuscator(cfg)
	})
	return obfuscator
}

// sqlConfig holds the config for the python SQL obfuscator.
type sqlConfig struct {
	// DBMS identifies the type of database management system (e.g. MySQL, Postgres, and SQL Server).
	DBMS string `json:"dbms"`
	// TableNames specifies whether the obfuscator should extract and return table names as SQL metadata when obfuscating.
	TableNames bool `json:"table_names"`
	// CollectCommands specifies whether the obfuscator should extract and return commands as SQL metadata when obfuscating.
	CollectCommands bool `json:"collect_commands"`
	// CollectComments specifies whether the obfuscator should extract and return comments as SQL metadata when obfuscating.
	CollectComments bool `json:"collect_comments"`
	// ReplaceDigits specifies whether digits in table names and identifiers should be obfuscated.
	ReplaceDigits bool `json:"replace_digits"`
	// KeepSQLAlias specifies whether or not to strip sql aliases while obfuscating.
	KeepSQLAlias bool `json:"keep_sql_alias"`
	// DollarQuotedFunc specifies whether or not to remove $func$ strings in postgres.
	DollarQuotedFunc bool `json:"dollar_quoted_func"`
	// ReturnJSONMetadata specifies whether the stub will return metadata as JSON.
	ReturnJSONMetadata bool `json:"return_json_metadata"`
}

// ObfuscateSQL obfuscates & normalizes the provided SQL query, writing the error into errResult if the operation
// fails. An optional configuration may be passed to change the behavior of the obfuscator.
//
//export ObfuscateSQL
func ObfuscateSQL(rawQuery, opts *C.char, errResult **C.char) *C.char {
	optStr := C.GoString(opts)
	if optStr == "" {
		// ensure we have a valid JSON string before unmarshalling
		optStr = "{}"
	}
	var sqlOpts sqlConfig
	if err := json.Unmarshal([]byte(optStr), &sqlOpts); err != nil {
		log.Errorf("Failed to unmarshal obfuscation options: %s", err.Error())
		*errResult = TrackedCString(err.Error())
	}
	s := C.GoString(rawQuery)
	obfuscatedQuery, err := lazyInitObfuscator().ObfuscateSQLStringWithOptions(s, &obfuscate.SQLConfig{
		DBMS:             sqlOpts.DBMS,
		TableNames:       sqlOpts.TableNames,
		CollectCommands:  sqlOpts.CollectCommands,
		CollectComments:  sqlOpts.CollectComments,
		ReplaceDigits:    sqlOpts.ReplaceDigits,
		KeepSQLAlias:     sqlOpts.KeepSQLAlias,
		DollarQuotedFunc: sqlOpts.DollarQuotedFunc,
	})
	if err != nil {
		// memory will be freed by caller
		*errResult = TrackedCString(err.Error())
		return nil
	}
	if sqlOpts.ReturnJSONMetadata {
		out, err := json.Marshal(obfuscatedQuery)
		if err != nil {
			// memory will be freed by caller
			*errResult = TrackedCString(err.Error())
			return nil
		}
		// memory will be freed by caller
		return TrackedCString(string(out))
	}
	// memory will be freed by caller
	return TrackedCString(obfuscatedQuery.Query)
}

// ObfuscateSQLExecPlan obfuscates the provided json query execution plan, writing the error into errResult if the
// operation fails
//
//export ObfuscateSQLExecPlan
func ObfuscateSQLExecPlan(jsonPlan *C.char, normalize C.bool, errResult **C.char) *C.char {
	obfuscatedJSONPlan, err := lazyInitObfuscator().ObfuscateSQLExecPlan(
		C.GoString(jsonPlan),
		bool(normalize),
	)
	if err != nil {
		// memory will be freed by caller
		*errResult = TrackedCString(err.Error())
		return nil
	}
	// memory will be freed by caller
	return TrackedCString(obfuscatedJSONPlan)
}

// defaultSQLPlanNormalizeSettings are the default JSON obfuscator settings for both obfuscating and normalizing SQL
// execution plans
var defaultSQLPlanNormalizeSettings = obfuscate.JSONConfig{
	Enabled: true,
	ObfuscateSQLValues: []string{
		// mysql
		"attached_condition",
		// postgres
		"Cache Key",
		"Conflict Filter",
		"Function Call",
		"Filter",
		"Hash Cond",
		"Index Cond",
		"Join Filter",
		"Merge Cond",
		"Output",
		"Recheck Cond",
		"Repeatable Seed",
		"Sampling Parameters",
		"TID Cond",
	},
	KeepValues: []string{
		// mysql
		"access_type",
		"backward_index_scan",
		"cacheable",
		"delete",
		"dependent",
		"first_match",
		"key",
		"key_length",
		"possible_keys",
		"ref",
		"select_id",
		"table_name",
		"update",
		"used_columns",
		"used_key_parts",
		"using_MRR",
		"using_filesort",
		"using_index",
		"using_join_buffer",
		"using_temporary_table",
		// postgres
		"Actual Loops",
		"Actual Rows",
		"Actual Startup Time",
		"Actual Total Time",
		"Alias",
		"Async Capable",
		"Average Sort Space Used",
		"Cache Evictions",
		"Cache Hits",
		"Cache Misses",
		"Cache Overflows",
		"Calls",
		"Command",
		"Conflict Arbiter Indexes",
		"Conflict Resolution",
		"Conflicting Tuples",
		"Constraint Name",
		"CTE Name",
		"Custom Plan Provider",
		"Deforming",
		"Emission",
		"Exact Heap Blocks",
		"Execution Time",
		"Expressions",
		"Foreign Delete",
		"Foreign Insert",
		"Foreign Update",
		"Full-sort Group",
		"Function Name",
		"Generation",
		"Group Count",
		"Grouping Sets",
		"Group Key",
		"HashAgg Batches",
		"Hash Batches",
		"Hash Buckets",
		"Heap Fetches",
		"I/O Read Time",
		"I/O Write Time",
		"Index Name",
		"Inlining",
		"Join Type",
		"Local Dirtied Blocks",
		"Local Hit Blocks",
		"Local Read Blocks",
		"Local Written Blocks",
		"Lossy Heap Blocks",
		"Node Type",
		"Optimization",
		"Original Hash Batches",
		"Original Hash Buckets",
		"Parallel Aware",
		"Parent Relationship",
		"Partial Mode",
		"Peak Memory Usage",
		"Peak Sort Space Used",
		"Planned Partitions",
		"Planning Time",
		"Pre-sorted Groups",
		"Presorted Key",
		"Query Identifier",
		"Relation Name",
		"Rows Removed by Conflict Filter",
		"Rows Removed by Filter",
		"Rows Removed by Index Recheck",
		"Rows Removed by Join Filter",
		"Sampling Method",
		"Scan Direction",
		"Schema",
		"Settings",
		"Shared Dirtied Blocks",
		"Shared Hit Blocks",
		"Shared Read Blocks",
		"Shared Written Blocks",
		"Single Copy",
		"Sort Key",
		"Sort Method",
		"Sort Methods Used",
		"Sort Space Type",
		"Sort Space Used",
		"Strategy",
		"Subplan Name",
		"Subplans Removed",
		"Target Tables",
		"Temp Read Blocks",
		"Temp Written Blocks",
		"Time",
		"Timing",
		"Total",
		"Trigger",
		"Trigger Name",
		"Triggers",
		"Tuples Inserted",
		"Tuplestore Name",
		"WAL Bytes",
		"WAL FPI",
		"WAL Records",
		"Worker",
		"Worker Number",
		"Workers",
		"Workers Launched",
		"Workers Planned",
	},
}

// defaultSQLPlanObfuscateSettings builds upon sqlPlanNormalizeSettings by including cost & row estimates in the keep
// list
var defaultSQLPlanObfuscateSettings = obfuscate.JSONConfig{
	Enabled: true,
	KeepValues: append([]string{
		// mysql
		"cost_info",
		"filtered",
		"rows_examined_per_join",
		"rows_examined_per_scan",
		"rows_produced_per_join",
		// postgres
		"Plan Rows",
		"Plan Width",
		"Startup Cost",
		"Total Cost",
	}, defaultSQLPlanNormalizeSettings.KeepValues...),
	ObfuscateSQLValues: defaultSQLPlanNormalizeSettings.ObfuscateSQLValues,
}

//export getProcessStartTime
func getProcessStartTime() float64 {
	return float64(config.StartTime.Unix())
}
