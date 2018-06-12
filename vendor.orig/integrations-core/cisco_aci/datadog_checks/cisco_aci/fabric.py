# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

from . import metrics as aci_metrics
from . import helpers


class Fabric:
    """
    Collect fabric metrics from the APIC
    """

    def __init__(self, check, api, instance):
        self.check = check
        self.api = api
        self.instance = instance
        self.check_tags = check.check_tags

        # grab some functions from the check
        self.gauge = check.gauge
        self.rate = check.rate
        self.log = check.log
        self.submit_metrics = check.submit_metrics
        self.tagger = self.check.tagger
        self.external_host_tags = self.check.external_host_tags

    def collect(self):
        fabric_pods = self.api.get_fabric_pods()
        fabric_nodes = self.api.get_fabric_nodes()
        self.log.info("{} pods and {} nodes computed".format(len(fabric_nodes), len(fabric_pods)))
        pods = self.submit_pod_health(fabric_pods)
        self.submit_nodes_health(fabric_nodes, pods)

    def submit_pod_health(self, pods):
        pods_dict = {}
        for p in pods:
            pod = p.get('fabricPod', {})
            pod_attrs = pod.get('attributes', {})
            pod_id = pod_attrs.get('id')
            pods_dict[pod_id] = pod_attrs
            self.log.info("processing pod {}".format(pod_attrs['id']))
            tags = self.tagger.get_fabric_tags(p, 'fabricPod')
            stats = self.api.get_pod_stats(pod_id)
            self.submit_fabric_metric(stats, tags, 'fabricPod')
            self.log.info("finished processing pod {}".format(pod_attrs['id']))

        return pods_dict

    def submit_nodes_health(self, nodes, pods):
        for n in nodes:
            hostname = helpers.get_fabric_hostname(n)

            user_tags = self.instance.get('tags', [])
            tags = self.tagger.get_fabric_tags(n, 'fabricNode')
            self.external_host_tags[hostname] = tags + self.check_tags + user_tags

            node = n.get('fabricNode', {})
            node_attrs = node.get('attributes', {})
            node_id = node_attrs.get('id', {})

            pod_id = helpers.get_pod_from_dn(node_attrs['dn'])

            self.log.info("processing node {} on pod {}".format(node_id, pod_id))
            self.submit_process_metric(n, tags + self.check_tags + user_tags, hostname=hostname)
            if node_attrs['role'] != "controller":
                stats = self.api.get_node_stats(pod_id, node_id)
                self.submit_fabric_metric(stats, tags, 'fabricNode', hostname=hostname)
                self.process_eth(node_attrs)
            self.log.info("finished processing node {}".format(node_id))

    def process_eth(self, node):
        self.log.info("processing ethernet ports for {}".format(node['id']))
        hostname = helpers.get_fabric_hostname(node)
        pod_id = helpers.get_pod_from_dn(node['dn'])
        eth_list = self.api.get_eth_list(pod_id, node['id'])
        for e in eth_list:
            eth_attrs = helpers.get_attributes(e)
            eth_id = eth_attrs['id']
            tags = self.tagger.get_fabric_tags(e, 'l1PhysIf')
            stats = self.api.get_eth_stats(pod_id, node['id'], eth_id)
            self.submit_fabric_metric(stats, tags, 'l1PhysIf', hostname=hostname)
        self.log.info("finished processing ethernet ports for {}".format(node['id']))

    def submit_fabric_metric(self, stats, tags, obj_type, hostname=None):
        for s in stats:
            name = s.keys()[0]
            # we only want to collect the 5 minutes metrics.
            if '15min' in name or '5min' not in name:
                continue
            attrs = s[name]['attributes']
            if 'index' in attrs:
                continue

            metrics = {}
            for n, ms in aci_metrics.FABRIC_METRICS.iteritems():
                if n not in name:
                    continue
                for cisco_metric, dd_metric in ms.iteritems():
                    mname = dd_metric.format(self.get_fabric_type(obj_type))
                    mval = s.get(name, {}).get("attributes", {}).get(cisco_metric)
                    json_attrs = s.get(name, {}).get("attributes", {})
                    if mval and helpers.check_metric_can_be_zero(cisco_metric, mval, json_attrs):
                        metrics[mname] = mval

            self.submit_metrics(metrics, tags, hostname=hostname, instance=self.instance)

    def submit_process_metric(self, obj, tags, hostname=None):
        attrs = helpers.get_attributes(obj)
        node_id = helpers.get_node_from_dn(attrs['dn'])
        pod_id = helpers.get_pod_from_dn(attrs['dn'])

        if attrs['role'] == "controller":
            metrics = self.api.get_controller_proc_metrics(pod_id, node_id)
        else:
            metrics = self.api.get_spine_proc_metrics(pod_id, node_id)

        for d in metrics:
            if d.get("procCPUHist5min", {}).get('attributes'):
                data = d.get("procCPUHist5min").get("attributes", {})
                if data.get('index') == '0':
                    value = data.get('currentAvg')
                    if value:
                        self.gauge('cisco_aci.fabric.node.cpu.avg', value, tags=tags, hostname=hostname)
                    value = data.get('currentMax')
                    if value:
                        self.gauge('cisco_aci.fabric.node.cpu.max', value, tags=tags, hostname=hostname)
                    value = data.get('currentMin')
                    if value:
                        self.gauge('cisco_aci.fabric.node.cpu.min', value, tags=tags, hostname=hostname)

            if d.get("procSysCPUHist5min", {}).get('attributes'):
                data = d.get("procSysCPUHist5min").get("attributes", {})
                value = data.get('idleMax')
                if value:
                    self.gauge('cisco_aci.fabric.node.cpu.idle.max', value, tags=tags, hostname=hostname)
                    not_idle = 100.0 - float(value)
                    self.gauge('cisco_aci.fabric.node.cpu.max', not_idle, tags=tags, hostname=hostname)
                value = data.get('idleMin')
                if value:
                    self.gauge('cisco_aci.fabric.node.cpu.idle.min', value, tags=tags, hostname=hostname)
                    not_idle = 100.0 - float(value)
                    self.gauge('cisco_aci.fabric.node.cpu.min', not_idle, tags=tags, hostname=hostname)
                value = data.get('idleAvg')
                if value:
                    self.gauge('cisco_aci.fabric.node.cpu.idle.avg', value, tags=tags, hostname=hostname)
                    not_idle = 100.0 - float(value)
                    self.gauge('cisco_aci.fabric.node.cpu.avg', not_idle, tags=tags, hostname=hostname)

            if d.get("procMemHist5min", {}).get('attributes'):
                data = d.get("procMemHist5min").get("attributes", {})
                if data.get('index') == '0':
                    value = data.get('currentAvg')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.avg', value, tags=tags, hostname=hostname)
                    value = data.get('currentMax')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.max', value, tags=tags, hostname=hostname)
                    value = data.get('currentMin')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.min', value, tags=tags, hostname=hostname)

            if d.get("procSysMemHist5min", {}).get('attributes'):
                data = d.get("procSysMemHist5min").get("attributes", {})
                if data.get('index') == '0':
                    value = data.get('usedAvg')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.avg', value, tags=tags, hostname=hostname)
                    value = data.get('usedMax')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.max', value, tags=tags, hostname=hostname)
                    value = data.get('usedMin')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.min', value, tags=tags, hostname=hostname)

                    value = data.get('freeAvg')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.free.avg', value, tags=tags, hostname=hostname)
                    value = data.get('freeMax')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.free.max', value, tags=tags, hostname=hostname)
                    value = data.get('freeMin')
                    if value:
                        self.gauge('cisco_aci.fabric.node.mem.free.min', value, tags=tags, hostname=hostname)

    def get_fabric_type(self, obj_type):
        if obj_type == 'fabricNode':
            return 'node'
        if obj_type == 'fabricPod':
            return 'pod'
        if obj_type == 'l1PhysIf':
            return 'port'
