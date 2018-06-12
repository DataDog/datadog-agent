# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

import re

from datadog_checks.utils.containers import hash_mutable

from . import helpers


class CiscoTags:
    def __init__(self, check):
        self.check = check
        self.tenant_farbic_mapper = {}
        self.tenant_tags = {}
        self._api = None

    def app_tags(self, app):
        tags = []
        attrs = app.get('attributes', {})
        app_name = attrs.get('name')
        dn = attrs.get('dn')
        if app_name:
            tags.append("application:" + app_name)
        if dn:
            tenant = re.search('/tn-([a-zA-Z-_0-9]+)/', dn)
            if tenant:
                tags.append("tenant:" + tenant.group(1))
        return tags

    def tenant_mapper(self, edpt):
        tags = []
        attrs = edpt.get('attributes', {})
        epg_name = attrs.get('name')
        dn = attrs.get('dn')
        application_meta = [
            "endpoint_group:" + epg_name
        ]
        if dn:
            tenant = re.search('/tn-([a-zA-Z-_0-9]+)/', dn)
            if tenant:
                tenant_name = tenant.group(1)
                application_meta.append("tenant:" + tenant_name)
        if dn:
            app = re.search('/ap-([a-zA-Z-_0-9]+)/', dn)
            if app:
                app_name = app.group(1)
                application_meta.append("application:" + app_name)
        # adding meta tags
        meta = self.api.get_epg_meta(tenant_name, app_name, epg_name)
        endpoint_meta = []
        if len(meta) > 0:
            meta = meta[0]
            meta_attrs = meta.get('fvCEp', {}).get('attributes')
            if meta_attrs:
                ip = meta_attrs.get('ip')
                if ip:
                    endpoint_meta.append("ip:" + ip)
                mac = meta_attrs.get('mac')
                if mac:
                    endpoint_meta.append("mac:" + mac)
                encap = meta_attrs.get('encap')
                if encap:
                    endpoint_meta.append("encap:" + encap)
                # adding application tags
        endpoint_meta += application_meta

        context_hash = hash_mutable(endpoint_meta)
        eth_meta = []
        if self.tenant_tags.get(context_hash):
            eth_meta = self.tenant_tags.get(context_hash)
        else:
            # adding eth and node tags
            eth_list = self.api.get_eth_list_for_epg(tenant_name, app_name, epg_name)
            for eth in eth_list:
                eth_attrs = eth.get('fvRsCEpToPathEp', {}).get('attributes', {})
                port = re.search('/pathep-\[(.+?)\]', eth_attrs.get('tDn', ''))
                if not port:
                    continue
                eth_tag = 'port:' + port.group(1)
                if eth_tag not in eth_meta:
                    eth_meta.append(eth_tag)
                node = re.search('/paths-(.+?)/', eth_attrs.get('tDn', ''))
                if not node:
                    continue
                eth_node = 'node_id:' + node.group(1)
                if eth_node not in eth_meta:
                    eth_meta.append(eth_node)
                # populating the map for eth-app mapping

                tenant_fabric_key = node.group(1) + ":" + port.group(1)
                if tenant_fabric_key not in self.tenant_farbic_mapper:
                    self.tenant_farbic_mapper[tenant_fabric_key] = application_meta
                else:
                    self.tenant_farbic_mapper[tenant_fabric_key].extend(application_meta)

                self.tenant_farbic_mapper[tenant_fabric_key] = list(set(self.tenant_farbic_mapper[tenant_fabric_key]))

        tags = tags + endpoint_meta + eth_meta
        if len(eth_meta) > 0:
            self.log.debug('adding eth level tags: %s' % eth_meta)
        return tags

    def get_tags(self, obj, obj_type):
        tags = []
        if obj_type == 'endpoint_group':
            tags = self.tenant_mapper(obj)
        if obj_type == 'tenant':
            tags = ["tenant:" + obj]
        if obj_type == 'application':
            tags = self.app_tags(obj)
        return tags

    def get_fabric_tags(self, obj, obj_type):
        tags = []
        obj = helpers.get_attributes(obj)
        if obj_type == 'fabricNode':
            if obj.get('role') != "controller":
                tags.append("switch_role:" + obj.get('role'))
            tags.append("apic_role:" + obj.get('role'))
            tags.append("node_id:" + obj.get('id'))
            tags.append("fabric_state:" + obj.get('fabricSt'))
            tags.append("fabric_pod_id:" + helpers.get_pod_from_dn(obj.get('dn')))
        if obj_type == 'fabricPod':
            tags.append("fabric_pod_id:" + obj['id'])
        if obj_type == 'l1PhysIf':
            tags.append("port:" + obj.get('id'))
            if obj.get('medium'):
                tags.append("medium:" + obj.get('medium'))
            if obj.get('snmpTrapSt'):
                tags.append("snmpTrapSt:" + obj.get('snmpTrapSt'))
            node_id = helpers.get_node_from_dn(obj.get('dn'))
            pod_id = helpers.get_pod_from_dn(obj.get('dn'))
            tags.append("node_id:" + node_id)
            tags.append("fabric_pod_id:" + pod_id)
            key = node_id + ":" + obj.get('id')
            if key in self.tenant_farbic_mapper.keys():
                tags = tags + self.tenant_farbic_mapper[key]
        return tags

    @property
    def log(self):
        return self.check.log

    @property
    def api(self):
        return self._api

    @api.setter
    def api(self, value):
        self._api = value
