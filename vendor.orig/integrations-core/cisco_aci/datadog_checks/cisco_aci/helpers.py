# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

import re

POD_REGEX = re.compile('pod-([0-9]+)')
BD_REGEX = re.compile('/BD-([^/]+)/')
APP_REGEX = re.compile('/ap-([^/]+)/')
CEP_REGEX = re.compile('/cep-([^/]+)/')
EPG_REGEX = re.compile('/epg-([^/]+)/')
IP_REGEX = re.compile('/ip-([^/]+)/')
NODE_REGEX = re.compile('node-([0-9]+)')


def parse_capacity_tags(dn):
    """
    This parses tags from a dn designator. They look like this:
    topology/pod-1/node-101/sys/phys-[eth1/6]/CDeqptMacsectxpkts5min
    """
    tags = []
    pod = get_pod_from_dn(dn)
    if pod:
        tags.append("fabric_pod_id:{}".format(pod))
    node = get_node_from_dn(dn)
    if node:
        tags.append("node_id:{}".format(node))

    return tags


def get_pod_from_dn(dn):
    """
    This parses the pod from a dn designator. They look like this:
    topology/pod-1/node-101/sys/phys-[eth1/6]/CDeqptMacsectxpkts5min
    """
    pod = POD_REGEX.search(dn)
    if pod:
        return pod.group(1)
    else:
        return None


def get_bd_from_dn(dn):
    """
    This parses the bd from the dn designator. They look like this:
    topology/pod-1/node-101/sys/phys-[eth1/6]/CDeqptMacsectxpkts5min
    """
    bd = BD_REGEX.search(dn)
    if bd:
        return bd.group(1)
    else:
        return None


def get_app_from_dn(dn):
    """
    This parses the app from the dn designator. They look like this:
    uni/tn-DataDog/ap-DtDg-AP1-EcommerceApp/epg-DtDg-Ecomm/HDl2IngrPktsAg1h
    """
    app = APP_REGEX.search(dn)
    if app:
        return app.group(1)
    else:
        return None


def get_cep_from_dn(dn):
    """
    This parses the cep from the dn designator. They look like this:
    uni/tn-DataDog/ap-DtDg-AP1-EcommerceApp/epg-DtDg-Ecomm/HDl2IngrPktsAg1h
    """
    cep = CEP_REGEX.search(dn)
    if cep:
        return cep.group(1)
    else:
        return None


def get_epg_from_dn(dn):
    """
    This parses the epg from the dn designator. They look like this:
    uni/tn-DataDog/ap-DtDg-AP1-EcommerceApp/epg-DtDg-Ecomm/HDl2IngrPktsAg1h
    """
    epg = EPG_REGEX.search(dn)
    if epg:
        return epg.group(1)
    else:
        return None


def get_ip_from_dn(dn):
    """
    This parses the ip from the dn designator. They look like this:
    uni/tn-DataDog/ap-DtDg-AP1-EcommerceApp/epg-DtDg-Ecomm/HDl2IngrPktsAg1h
    """
    ip = IP_REGEX.search(dn)
    if ip:
        return ip.group(1)
    else:
        return None


def get_event_tags_from_dn(dn):
    """
    This grabs the event tags from the dn designator. They look like this:
    uni/tn-DataDog/ap-DtDg-AP1-EcommerceApp/epg-DtDg-Ecomm/HDl2IngrPktsAg1h
    """
    tags = []
    node = get_node_from_dn(dn)
    if node:
        tags.append("node:" + node)
    app = get_app_from_dn(dn)
    if app:
        tags.append("app:" + app)
    bd = get_bd_from_dn(dn)
    if bd:
        tags.append("bd:" + bd)
    cep = get_cep_from_dn(dn)
    if cep:
        tags.append("mac:" + cep)
    ip = get_ip_from_dn(dn)
    if ip:
        tags.append("ip:" + ip)
    epg = get_epg_from_dn(dn)
    if epg:
        tags.append("epg:" + epg)
    return tags


def get_node_from_dn(dn):
    """
    This parses the node from a dn designator. They look like this:
    topology/pod-1/node-101/sys/phys-[eth1/6]/CDeqptMacsectxpkts5min
    """
    node = NODE_REGEX.search(dn)
    if node:
        return node.group(1)
    else:
        return None


def get_hostname_from_dn(dn):
    """
    This parses the hostname from a dn designator. They look like this:
    topology/pod-1/node-101/sys/phys-[eth1/6]/CDeqptMacsectxpkts5min
    """
    pod = get_pod_from_dn(dn)
    node = get_node_from_dn(dn)
    return get_hostname(pod, node)


def get_hostname(pod, node):
    """
    This puts together the hostname in a way that is consistent
    """
    if pod and node:
        return "pod-{}-node-{}".format(pod, node)
    else:
        return None


def get_fabric_hostname(obj):
    """
    This grabs the hostname from the object
    The object looks something like this:
    {
    "dn": "topology/pod-1/node-101/sys/phys-[eth1/6]/CDeqptMacsectxpkts5min"
    ...
    }
    """
    attrs = get_attributes(obj)
    dn = attrs['dn']

    return get_hostname_from_dn(dn)


def get_attributes(obj):
    """
    the json objects look like this:
    {
    "objType": {
      "attributes": {
      ...
      }
    }
    It always has the attributes nested below the object type
    This helper provides a way of getting at the attributes
    """
    if obj.get('imdata'):
        obj = obj.get('imdata')
    keys = obj.keys()
    if len(keys) > 0:
        key = keys[0]
    else:
        return {}
    key_obj = obj.get(key, {})
    if type(key_obj) is not dict:
        # if the object is not a dict
        # it is probably already scoped to attributes
        return obj
    attrs = key_obj.get('attributes')
    if not attrs:
        # if the attributes doesn't exist,
        # it is probably already scoped to attributes
        return obj
    return attrs


def check_metric_can_be_zero(metric_name, metric_value, json_attributes):
    """
    When a counter is reset, don't send a zero because it will look bad on the graphs
    This checks if the zero makes sense or not
    """
    if "last" in metric_name or "Last" in metric_name:
        return True
    if not metric_value:
        return False
    if metric_value == 0 or metric_value == "0" or metric_value == "0.000000" or float(metric_value) == 0.0:
        if not json_attributes.get('cnt'):
            return False
        if json_attributes.get('cnt') == "0" or json_attributes.get('cnt') == 0:
            return False
    return True
