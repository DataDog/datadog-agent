# stdlib
import logging
import socket
import struct
from urlparse import urljoin

# project
from utils.http import retrieve_json

DEFAULT_METHOD = 'http'
METRICS_PATH = '/api/v1.3/subcontainers/'
DEFAULT_CADVISOR_PORT = 4194
DEFAULT_KUBELET_PORT = 10255
DEFAULT_MASTER_PORT = 8080

log = logging.getLogger('collector')
_kube_settings = {}

def get_kube_settings():
    global _kube_settings
    return _kube_settings


def set_kube_settings(instance):
    global _kube_settings

    host = instance.get("host") or _get_default_router()
    cadvisor_port = instance.get('port', DEFAULT_CADVISOR_PORT)
    method = instance.get('method', DEFAULT_METHOD)
    metrics_url = urljoin('%s://%s:%d' % (method, host, cadvisor_port), METRICS_PATH)
    kubelet_port = instance.get('kubelet_port', DEFAULT_KUBELET_PORT)
    master_port = instance.get('master_port', DEFAULT_MASTER_PORT)
    master_host = instance.get('master_host', host)

    _kube_settings = {
        "host": host,
        "method": method,
        "metrics_url": metrics_url,
        "cadvisor_port": cadvisor_port,
        "labels_url": '%s://%s:%d/pods' % (method, host, kubelet_port),
        "master_url_nodes": '%s://%s:%d/api/v1/nodes' % (method, master_host, master_port),
        "kube_health_url": '%s://%s:%d/healthz' % (method, host, kubelet_port)
    }

    return _kube_settings


def get_kube_labels():
    global _kube_settings
    pods = retrieve_json(_kube_settings["labels_url"])
    kube_labels = {}
    for pod in pods["items"]:
        metadata = pod.get("metadata", {})
        name = metadata.get("name")
        namespace = metadata.get("namespace")
        labels = metadata.get("labels")
        if name and labels and namespace:
            key = "%s/%s" % (namespace, name)
            kube_labels[key] = ["kube_%s:%s" % (k,v) for k,v in labels.iteritems()]

    return kube_labels

def _get_default_router():
    try:
        with open('/proc/net/route') as f:
            for line in f.readlines():
                fields = line.strip().split()
                if fields[1] == '00000000':
                    return socket.inet_ntoa(struct.pack('<L', int(fields[2], 16)))
    except IOError, e:
        log.error('Unable to open /proc/net/route: %s', e)

    return None
