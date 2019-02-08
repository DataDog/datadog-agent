// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_BUILTINS_H
#define DATADOG_AGENT_SIX_BUILTINS_H

#include <string>

namespace builtins {
    enum ExtensionModule {
        MODULE__UTIL = 0,
        MODULE_AGGREGATOR,
        MODULE_CONTAINERS,
        MODULE_DATADOG_AGENT,
        MODULE_KUBEUTIL,
        MODULE_TAGGER,
        MODULE_UTIL,
    };

    // these strings need to be alive for the whole interpreter lifetime because
    // they'll be used from the CPython Inittab
    static std::string module_unknown = "";
    static std::string module__util = "_util";
    static std::string module_aggregator = "aggregator";
    static std::string module_containers = "containers";
    static std::string module_datadog_agent = "datadog_agent";
    static std::string module_kubeutil = "kubeutil";
    static std::string module_tagger = "tagger";
    static std::string module_util = "util";

    // we return a reference to the static strings so CPython is happy (see above comment)
    const std::string &getExtensionModuleName(ExtensionModule m);
}

#endif
