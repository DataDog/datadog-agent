# (C) Datadog, Inc. 2015-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
from collections import defaultdict
import logging
import os
from urlparse import urljoin
from urllib import urlencode

# project
from util import check_yaml
from utils.checkfiles import get_conf_path
from utils.http import retrieve_json
from utils.singleton import Singleton
from utils.dockerutil import DockerUtil

import requests

log = logging.getLogger('collector')

KUBERNETES_CHECK_NAME = 'kubernetes'

DEFAULT_TLS_VERIFY = True


class KubeUtil:
    __metaclass__ = Singleton

    DEFAULT_METHOD = 'http'
    KUBELET_HEALTH_PATH = '/healthz'
    MACHINE_INFO_PATH = '/api/v1.3/machine/'
    METRICS_PATH = '/api/v1.3/subcontainers/'
    PODS_LIST_PATH = '/pods/'
    DEFAULT_CADVISOR_PORT = 4194
    DEFAULT_HTTP_KUBELET_PORT = 10255
    DEFAULT_HTTPS_KUBELET_PORT = 10250
    DEFAULT_MASTER_PORT = 8080
    DEFAULT_MASTER_NAME = 'kubernetes'  # DNS name to reach the master from a pod.
    CA_CRT_PATH = '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'
    AUTH_TOKEN_PATH = '/var/run/secrets/kubernetes.io/serviceaccount/token'

    POD_NAME_LABEL = "io.kubernetes.pod.name"
    NAMESPACE_LABEL = "io.kubernetes.pod.namespace"

    def __init__(self, instance=None):
        self.docker_util = DockerUtil()
        if instance is None:
            try:
                config_file_path = get_conf_path(KUBERNETES_CHECK_NAME)
                check_config = check_yaml(config_file_path)
                instance = check_config['instances'][0]
            # kubernetes.yaml was not found
            except IOError as ex:
                log.error(ex.message)
                instance = {}
            except Exception:
                log.error('Kubernetes configuration file is invalid. '
                          'Trying connecting to kubelet with default settings anyway...')
                instance = {}

        self.method = instance.get('method', KubeUtil.DEFAULT_METHOD)
        self._node_ip = self._node_name = None  # lazy evaluation
        self.host_name = os.environ.get('HOSTNAME')
        self.tls_settings = self._init_tls_settings(instance)

        # apiserver
        self.kubernetes_api_url = 'https://%s/api/v1' % (os.environ.get('KUBERNETES_SERVICE_HOST') or self.DEFAULT_MASTER_NAME)

        # kubelet
        try:
            self.kubelet_api_url = self._locate_kubelet(instance)
            if not self.kubelet_api_url:
                raise Exception("Couldn't find a method to connect to kubelet.")
        except Exception as ex:
            log.error("Kubernetes check exiting, cannot run without access to kubelet.")
            raise ex

        self.kubelet_host = self.kubelet_api_url.split(':')[1].lstrip('/')
        self.pods_list_url = urljoin(self.kubelet_api_url, KubeUtil.PODS_LIST_PATH)
        self.kube_health_url = urljoin(self.kubelet_api_url, KubeUtil.KUBELET_HEALTH_PATH)

        # cadvisor
        self.cadvisor_port = instance.get('port', KubeUtil.DEFAULT_CADVISOR_PORT)
        self.cadvisor_url = '%s://%s:%d' % (self.method, self.kubelet_host, self.cadvisor_port)
        self.metrics_url = urljoin(self.cadvisor_url, KubeUtil.METRICS_PATH)
        self.machine_info_url = urljoin(self.cadvisor_url, KubeUtil.MACHINE_INFO_PATH)

        # keep track of the latest k8s event we collected and posted
        # default value is 0 but TTL for k8s events is one hour anyways
        self.last_event_collection_ts = 0

    def _init_tls_settings(self, instance):
        """
        Initialize TLS settings for connection to apiserver and kubelet.
        """
        tls_settings = {}

        # apiserver
        client_crt = instance.get('apiserver_client_crt')
        client_key = instance.get('apiserver_client_key')
        apiserver_cacert = instance.get('apiserver_ca_cert')

        if client_crt and client_key and os.path.exists(client_crt) and os.path.exists(client_key):
            tls_settings['apiserver_client_cert'] = (client_crt, client_key)

        if apiserver_cacert and os.path.exists(apiserver_cacert):
            tls_settings['apiserver_cacert'] = apiserver_cacert

        token = self.get_auth_token()
        if token:
            tls_settings['bearer_token'] = token

        # kubelet
        kubelet_client_crt = instance.get('kubelet_client_crt')
        kubelet_client_key = instance.get('kubelet_client_key')
        if kubelet_client_crt and kubelet_client_key and os.path.exists(kubelet_client_crt) and os.path.exists(kubelet_client_key):
            tls_settings['kubelet_client_cert'] = (kubelet_client_crt, kubelet_client_key)

        cert = instance.get('kubelet_cert')
        if cert:
            tls_settings['kubelet_verify'] = cert
        else:
            tls_settings['kubelet_verify'] = instance.get('kubelet_tls_verify', DEFAULT_TLS_VERIFY)

        return tls_settings

    def _locate_kubelet(self, instance):
        """
        Kubelet may or may not accept un-authenticated http requests.
        If it doesn't we need to use its HTTPS API that may or may not
        require auth.
        """
        host = os.environ.get('KUBERNETES_KUBELET_HOST') or instance.get("host")
        if not host:
            # if no hostname was provided, use the docker hostname if cert
            # validation is not required, the kubernetes hostname otherwise.
            docker_hostname = self.docker_util.get_hostname(should_resolve=True)
            if self.tls_settings.get('kubelet_verify'):
                try:
                    k8s_hostname = self.get_node_hostname(docker_hostname)
                    host = k8s_hostname or docker_hostname
                except Exception as ex:
                    log.error(str(ex))
                    host = docker_hostname
            else:
                host = docker_hostname
        try:
            # check if the no-auth endpoint is enabled
            port = instance.get('kubelet_port', KubeUtil.DEFAULT_HTTP_KUBELET_PORT)
            no_auth_url = 'http://%s:%s' % (host, port)
            test_url = urljoin(no_auth_url, KubeUtil.KUBELET_HEALTH_PATH)
            self.perform_kubelet_query(test_url)
            return no_auth_url
        except Exception:
            log.debug("Couldn't query kubelet over HTTP, assuming it's not in no_auth mode.")

        port = instance.get('kubelet_port', KubeUtil.DEFAULT_HTTPS_KUBELET_PORT)

        https_url = 'https://%s:%s' % (host, port)
        test_url = urljoin(https_url, KubeUtil.KUBELET_HEALTH_PATH)
        self.perform_kubelet_query(test_url)

        return https_url

    def get_node_hostname(self, host):
        """
        Query the API server for the kubernetes hostname of the node
        using the docker hostname as a filter.
        """
        node_filter = {'labelSelector': 'kubernetes.io/hostname=%s' % host}
        node = self.retrieve_json_auth(
            self.kubernetes_api_url + '/nodes?%s' % urlencode(node_filter)
        )
        if len(node['items']) != 1:
            log.error('Error while getting node hostname: expected 1 node, got %s.' % len(node['items']))
        else:
            addresses = (node or {}).get('items', [{}])[0].get('status', {}).get('addresses', [])
            for address in addresses:
                if address.get('type') == 'Hostname':
                    return address['address']
        return None

    def get_kube_labels(self, excluded_keys=None):
        pods = self.retrieve_pods_list()
        return self.extract_kube_labels(pods, excluded_keys=excluded_keys)

    def extract_kube_labels(self, pods_list, excluded_keys=None):
        """
        Extract labels from a list of pods coming from
        the kubelet API.
        """
        excluded_keys = excluded_keys or []
        kube_labels = defaultdict(list)
        pod_items = pods_list.get("items") or []
        for pod in pod_items:
            metadata = pod.get("metadata", {})
            name = metadata.get("name")
            namespace = metadata.get("namespace")
            labels = metadata.get("labels")
            if name and labels and namespace:
                key = "%s/%s" % (namespace, name)

                for k, v in labels.iteritems():
                    if k in excluded_keys:
                        continue

                    kube_labels[key].append(u"kube_%s:%s" % (k, v))

        return kube_labels

    def retrieve_pods_list(self):
        """
        Retrieve the list of pods for this cluster querying the kubelet API.

        TODO: the list of pods could be cached with some policy to be decided.
        """
        return self.perform_kubelet_query(self.pods_list_url).json()

    def retrieve_machine_info(self):
        """
        Retrieve machine info from Cadvisor.
        """
        return retrieve_json(self.machine_info_url)

    def retrieve_metrics(self):
        """
        Retrieve metrics from Cadvisor.
        """
        return retrieve_json(self.metrics_url)

    def perform_kubelet_query(self, url, verbose=True, timeout=10):
        """
        Perform and return a GET request against kubelet. Support auth and TLS validation.
        """
        tls_context = self.tls_settings

        headers = None
        cert = tls_context.get('kubelet_client_cert')
        verify = tls_context.get('kubelet_verify', DEFAULT_TLS_VERIFY)

        # if cert-based auth is enabled, don't use the token.
        if not cert and url.lower().startswith('https'):
            headers = {'Authorization': 'Bearer {}'.format(self.get_auth_token())}

        return requests.get(url, timeout=timeout, verify=verify,
            cert=cert, headers=headers, params={'verbose': verbose})

    def retrieve_json_auth(self, url, timeout=10, verify=None):
        """
        Kubernetes API requires authentication using a token available in
        every pod, or with a client X509 cert/key pair.
        We authenticate using the service account token by default
        and replace this behavior with cert authentication if the user provided
        a cert/key pair in the instance.

        We try to verify the server TLS cert if the public cert is available.
        """
        verify = self.tls_settings.get('apiserver_cacert')
        if not verify:
            verify = self.CA_CRT_PATH if os.path.exists(self.CA_CRT_PATH) else False
        log.debug('tls validation: {}'.format(verify))

        cert = self.tls_settings.get('apiserver_client_cert')
        bearer_token = self.tls_settings.get('bearer_token') if not cert else None
        headers = {'Authorization': 'Bearer {}'.format(bearer_token)} if bearer_token else None

        r = requests.get(url, timeout=timeout, headers=headers, verify=verify, cert=cert)
        r.raise_for_status()
        return r.json()

    def get_node_info(self):
        """
        Return the IP address and the hostname of the node where the pod is running.
        """
        if None in (self._node_ip, self._node_name):
            self._fetch_host_data()
        return self._node_ip, self._node_name

    def _fetch_host_data(self):
        """
        Retrieve host name and IP address from the payload returned by the listing
        pods endpoints from kubelet.

        The host IP address is different from the default router for the pod.
        """
        try:
            pod_items = self.retrieve_pods_list().get("items") or []
        except Exception as e:
            log.warning("Unable to retrieve pod list %s. Not fetching host data", str(e))
            return

        for pod in pod_items:
            metadata = pod.get("metadata", {})
            name = metadata.get("name")
            if name == self.host_name:
                status = pod.get('status', {})
                spec = pod.get('spec', {})
                # if not found, use an empty string - we use None as "not initialized"
                self._node_ip = status.get('hostIP', '')
                self._node_name = spec.get('nodeName', '')
                break

    def extract_event_tags(self, event):
        """
        Return a list of tags extracted from an event object
        """
        tags = []

        if 'reason' in event:
            tags.append('reason:%s' % event.get('reason', '').lower())
        if 'namespace' in event.get('metadata', {}):
            tags.append('namespace:%s' % event['metadata']['namespace'])
        if 'host' in event.get('source', {}):
            tags.append('node_name:%s' % event['source']['host'])
        if 'kind' in event.get('involvedObject', {}):
            tags.append('object_type:%s' % event['involvedObject'].get('kind', '').lower())

        return tags

    def are_tags_filtered(self, tags):
        """
        Because it is a pain to call it from the kubernetes check otherwise.
        """
        return self.docker_util.are_tags_filtered(tags)

    @classmethod
    def get_auth_token(cls):
        """
        Return a string containing the authorization token for the pod.
        """
        try:
            with open(cls.AUTH_TOKEN_PATH) as f:
                return f.read()
        except IOError as e:
            log.error('Unable to read token from {}: {}'.format(cls.AUTH_TOKEN_PATH, e))

        return None
