#include <builtins.h>

const std::string &builtins::getExtensionModuleName(ExtensionModule m) {
    using namespace builtins;

    switch (m) {
    case MODULE__UTIL:
        return module__util;
    case MODULE_AGGREGATOR:
        return module_aggregator;
    case MODULE_CONTAINERS:
        return module_containers;
    case MODULE_DATADOG_AGENT:
        return module_datadog_agent;
    case MODULE_KUBEUTIL:
        return module_kubeutil;
    case MODULE_TAGGER:
        return module_tagger;
    case MODULE_UTIL:
        return module_util;
    default:
        return module_unknown;
    }
}
