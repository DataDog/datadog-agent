// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_TYPES_H
#define DATADOG_AGENT_SIX_TYPES_H

#ifdef __cplusplus
extern "C" {
#endif

typedef enum six_module_e {
    DATADOG_AGENT_SIX__UTIL = 0,
    DATADOG_AGENT_SIX_AGGREGATOR,
    DATADOG_AGENT_SIX_CONTAINERS,
    DATADOG_AGENT_SIX_DATADOG_AGENT,
    DATADOG_AGENT_SIX_KUBEUTIL,
    DATADOG_AGENT_SIX_TAGGER,
    DATADOG_AGENT_SIX_UTIL,
} six_module_t;

// these strings need to be alive for the whole interpreter lifetime because
// they'll be used from the CPython Inittab. Be sure to keep these in sync
// with `six_module_e` contents.
static const char *datadog_agent_six_unknown = "";
static const char *datadog_agent_six__util = "_util";
static const char *datadog_agent_six_aggregator = "aggregator";
static const char *datadog_agent_six_containers = "containers";
static const char *datadog_agent_six_datadog_agent = "datadog_agent";
static const char *datadog_agent_six_kubeutil = "kubeutil";
static const char *datadog_agent_six_tagger = "tagger";
static const char *datadog_agent_six_util = "util";

#ifdef __cplusplus
}
#endif
#endif
