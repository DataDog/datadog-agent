# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

METRIC_PREFIX = "cisco_aci"

FABRIC_PREFIX = METRIC_PREFIX + ".fabric"

FABRIC_METRICS = {
    "fabricNodeHealth": {
        "healthLast": FABRIC_PREFIX + ".node.health.cur",
        "healthMax": FABRIC_PREFIX + ".node.health.max",
        "healthMin": FABRIC_PREFIX + ".node.health.min"
    },
    "fabricOverallHealth": {
        "healthLast": FABRIC_PREFIX + ".pod.health.cur",
        "healthMax": FABRIC_PREFIX + ".pod.health.max",
        "healthMin": FABRIC_PREFIX + ".pod.health.min"
    },
    "fvFltCounter": {
        "critcountLast": FABRIC_PREFIX + ".{}.fault_counter.crit",
        "warncountLast": FABRIC_PREFIX + ".{}.fault_counter.warn"
    },
    "eqptEgrTotal": {
        "pktsLast": FABRIC_PREFIX + ".{}.egr_total.pkts",
        "pktsRate": FABRIC_PREFIX + ".{}.egr_total.pkts.rate",
        "bytesCum": FABRIC_PREFIX + ".{}.egr_total.bytes.cum",
        "bytesRate": FABRIC_PREFIX + ".{}.egr_total.bytes.rate",
    },
    "eqptIngrTotal": {
        "pktsLast": FABRIC_PREFIX + ".{}.ingr_total.pkts",
        "pktsRate": FABRIC_PREFIX + ".{}.ingr_total.pkts.rate",
        "bytesCum": FABRIC_PREFIX + ".{}.ingr_total.bytes.cum",
        "bytesRate": FABRIC_PREFIX + ".{}.ingr_total.bytes.rate",
    },
    "eqptEgrDropPkts": {
        "bufferCum": FABRIC_PREFIX + ".{}.egr_drop_pkts.buffer.cum",
        "bufferLast": FABRIC_PREFIX + ".{}.egr_drop_pkts.buffer",
        "errorBase": FABRIC_PREFIX + ".{}.egr_drop_pkts.errors"
    },
    "eqptEgrBytes": {
        "multicastLast": FABRIC_PREFIX + ".{}.egr_bytes.multicast",
        "multicastCum": FABRIC_PREFIX + ".{}.egr_bytes.multicast.cum",
        "unicastLast": FABRIC_PREFIX + ".{}.egr_bytes.unicast",
        "unicastCum": FABRIC_PREFIX + ".{}.egr_bytes.unicast.cum",
        "floodLast": FABRIC_PREFIX + ".{}.egr_bytes.flood",
        "floodCum": FABRIC_PREFIX + ".{}.egr_bytes.flood.cum",
    },
    "eqptIngrBytes": {
        "multicastLast": FABRIC_PREFIX + ".{}.ingr_bytes.multicast",
        "multicastCum": FABRIC_PREFIX + ".{}.ingr_bytes.multicast.cum",
        "unicastLast": FABRIC_PREFIX + ".{}.ingr_bytes.unicast",
        "unicastCum": FABRIC_PREFIX + ".{}.ingr_bytes.unicast.cum",
        "floodLast": FABRIC_PREFIX + ".{}.ingr_bytes.flood",
        "floodCum": FABRIC_PREFIX + ".{}.ingr_bytes.flood.cum",
    },
}

TENANT_PREFIX = METRIC_PREFIX + ".tenant"
APPLICATION_PREFIX = TENANT_PREFIX + ".application"
ENDPOINT_GROUP_PREFIX = APPLICATION_PREFIX + ".endpoint"


def make_tenant_metrics():
    metrics = {
        "fvOverallHealth": {
             "healthAvg": "{}.overall_health",
             "healthLast": "{}.health"
        },
        "fvFltCounter": {
            "warncountAvg": "{}.fault_counter"
        }
    }

    endpoint_metrics = {
        "l2IngrPktsAg": {
            "floodCum": "{}.ingress_pkts.flood.cum",
            "dropCum": "{}.ingress_pkts.drop.cum",
            "unicastCum": "{}.ingress_pkts.unicast.cum",
            "unicastRate": "{}.ingress_pkts.unicast.rate",
            "multicastCum": "{}.ingress_pkts.multicast.cum",
            "multicastRate": "{}.ingress_pkts.multicast.rate"
        },
        "l2EgrPktsAg": {
            "floodCum": "{}.egress_pkts.flood.cum",
            "dropCum": "{}.egress_pkts.drop.cum",
            "unicastCum": "{}.egress_pkts.unicast.cum",
            "unicastRate": "{}.egress_pkts.unicast.rate",
            "multicastCum": "{}.egress_pkts.multicast.cum",
            "multicastRate": "{}.egress_pkts.multicast.rate"
        },
        "l2IngrBytesAg": {
            "floodCum": "{}.ingress_bytes.flood.cum",
            "dropCum": "{}.ingress_bytes.drop.cum",
            "unicastCum": "{}.ingress_bytes.unicast.cum",
            "unicastRate": "{}.ingress_bytes.unicast.rate",
            "multicastCum": "{}.ingress_bytes.multicast.cum",
            "multicastRate": "{}.ingress_bytes.multicast.rate"
        },
        "l2EgrBytesAg": {
            "floodCum": "{}.egress_bytes.flood.cum",
            "dropCum": "{}.egress_bytes.drop.cum",
            "unicastCum": "{}.egress_bytes.unicast.cum",
            "unicastRate": "{}.egress_bytes.unicast.rate",
            "multicastCum": "{}.egress_bytes.multicast.cum",
            "multicastRate": "{}.egress_bytes.multicast.rate"
        },
    }

    tenant_metrics = {
        "tenant": {},
        "application": {},
        "endpoint_group": {}
    }

    for cisco_metric, metric_map in metrics.iteritems():
        tenant_metrics["tenant"][cisco_metric] = {}
        tenant_metrics["application"][cisco_metric] = {}
        tenant_metrics["endpoint_group"][cisco_metric] = {}
        for sub_metric, dd_metric in metric_map.iteritems():
            dd_tenant_metric = dd_metric.format(TENANT_PREFIX)
            tenant_metrics["tenant"][cisco_metric][sub_metric] = dd_tenant_metric
            dd_app_metric = dd_metric.format(APPLICATION_PREFIX)
            tenant_metrics["application"][cisco_metric][sub_metric] = dd_app_metric
            dd_epg_metric = dd_metric.format(ENDPOINT_GROUP_PREFIX)
            tenant_metrics["endpoint_group"][cisco_metric][sub_metric] = dd_epg_metric

    for cisco_metric, metric_map in endpoint_metrics.iteritems():
        if not tenant_metrics.get("endpoint_group", {}).get(cisco_metric):
            tenant_metrics["endpoint_group"][cisco_metric] = {}
        for sub_metric, dd_metric in metric_map.iteritems():
            dd_epg_metric = dd_metric.format(TENANT_PREFIX)
            tenant_metrics["endpoint_group"][cisco_metric][sub_metric] = dd_epg_metric

    return tenant_metrics


# Some metrics will show zeroes only when the counter resets and that makes the metric's values problematic
# in that case it's preferable to submit nothing
METRICS_NO_ZEROES = [
    TENANT_PREFIX + ".overall_health"
]

CAPACITY_PREFIX = METRIC_PREFIX + ".capacity"
LEAF_CAPACITY_PREFIX = CAPACITY_PREFIX + ".leaf"

CAPACITY_CONTEXT_METRICS = {
    "l2BD": {
        "metric_name": LEAF_CAPACITY_PREFIX + ".bridge_domain",
        "limit_value": 3500,
    },
    "fvEpP": {
        "metric_name": LEAF_CAPACITY_PREFIX + ".endpoint_group",
        "limit_value": 3500,
    },
    "l3Dom": {
        "metric_name": LEAF_CAPACITY_PREFIX + ".vrf",
        "limit_value": 800,
    },
}

EQPT_CAPACITY_METRICS = {
    "eqptcapacityL3TotalUsageCap5min": {
        "v4TotalEpCapCum": LEAF_CAPACITY_PREFIX + ".ipv4_endpoint.limit",
        "v6TotalEpCapCum": LEAF_CAPACITY_PREFIX + ".ipv6_endpoint.limit",
    },
    "eqptcapacityL3TotalUsage5min": {
        "v4TotalEpCum": LEAF_CAPACITY_PREFIX + ".ipv4_endpoint.utilized",
        "v6TotalEpCum": LEAF_CAPACITY_PREFIX + ".ipv6_endpoint.utilized",
    },
    "eqptcapacityVlanUsage5min": {
        "totalCapCum": LEAF_CAPACITY_PREFIX + ".vlan.limit",
        "totalCum": LEAF_CAPACITY_PREFIX + ".vlan.utilized",
    },
    "eqptcapacityPolUsage5min": {
        "polUsageCapCum": LEAF_CAPACITY_PREFIX + ".policy_cam.limit",
        "polUsageCum": LEAF_CAPACITY_PREFIX + ".policy_cam.utilized",
    },
    "eqptcapacityMcastUsage5min": {
        "localEpCapCum": LEAF_CAPACITY_PREFIX + ".multicast.limit",
        "localEpCum": LEAF_CAPACITY_PREFIX + ".multicast.utilized",
    },
}

APIC_CAPACITY_PREFIX = CAPACITY_PREFIX + ".apic"
APIC_CAPACITY_LIMITS = {
    "fabricNode": APIC_CAPACITY_PREFIX + ".fabric_node.limit",
    "vzBrCP": APIC_CAPACITY_PREFIX + ".contract.limit",
    "fvTenant": APIC_CAPACITY_PREFIX + ".tenant.limit",
    "fvCEp": APIC_CAPACITY_PREFIX + ".endpoint.limit",
    "plannerVmwareDomainTmpl": APIC_CAPACITY_PREFIX + ".vmware_domain.limit",
    "fvCtx": APIC_CAPACITY_PREFIX + ".private_network.limit",
    "plannerAzureDomainTmpl": APIC_CAPACITY_PREFIX + ".azure_domain.limit",
    "plannerAzureDomain": APIC_CAPACITY_PREFIX + ".azure_domain.endpoint_group.limit",
    "vnsGraphInst": APIC_CAPACITY_PREFIX + ".service_graph.limit",
    "fvBD": APIC_CAPACITY_PREFIX + ".bridge_domain.limit",
    "fvAEPg": APIC_CAPACITY_PREFIX + ".endpoint_group.limit",
    "plannerVmwareDomain": APIC_CAPACITY_PREFIX + ".vmware_domain.endpoint_group.limit",
}

APIC_CAPACITY_METRICS = {
    "fvTenant": {
        "metric_name": APIC_CAPACITY_PREFIX + ".tenant.utilized",
    },
    "fvCtx": {
        "metric_name": APIC_CAPACITY_PREFIX + ".private_network.utilized",
    },
    "fvAEPg": {
        "metric_name": APIC_CAPACITY_PREFIX + ".endpoint_group.utilized",
    },
    "fvBD": {
        "metric_name": APIC_CAPACITY_PREFIX + ".bridge_domain.utilized",
    },
    "fvCEp": {
        "metric_name": APIC_CAPACITY_PREFIX + ".endpoint.utilized",
    },
    "fabricNode": {
        "query_string": 'query-target-filter=eq(fabricNode.role,"leaf")',
        "metric_name": APIC_CAPACITY_PREFIX + ".fabric_node.utilized",
        "type": "len"
    }
}
