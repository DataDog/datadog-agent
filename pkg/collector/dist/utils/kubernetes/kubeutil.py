# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# stdlib
from collections import defaultdict
import logging
import os
import simplejson as json
import time
from urllib import urlencode
from urlparse import urljoin

# project
from utils.check_yaml import check_yaml
from utils.checkfiles import get_conf_path
from utils.dockerutil import DockerUtil
from utils.http import retrieve_json
from utils.kubernetes import LeaderElector, KubeEventRetriever, PodServiceMapper
from utils.singleton import Singleton

import requests

log = logging.getLogger('collector')

KUBERNETES_CHECK_NAME = 'kubernetes'
DEFAULT_NAMESPACE = 'default'

DEFAULT_TLS_VERIFY = True

CREATOR_KIND_TO_TAG = {
    'DaemonSet': 'kube_daemon_set',
    'ReplicaSet': 'kube_replica_set',
    'ReplicationController': 'kube_replication_controller',
    'StatefulSet': 'kube_stateful_set',
    'Deployment': 'kube_deployment',
    'Job': 'kube_job'
}

DEFAULT_INIT_RETRIES = 0
DEFAULT_RETRY_INTERVAL = 20  # seconds


def detect_is_k8s():
    """
    Logic for DockerUtil to detect whether to enable Kubernetes code paths
    It check whether we have a KUBERNETES_PORT environment variable (running
    in a pod) or a valid kubernetes.yaml conf file
    """
    if 'KUBERNETES_PORT' in os.environ:
        return True
    else:
        try:
            k8_config_file_path = get_conf_path(KUBERNETES_CHECK_NAME)
            k8_check_config = check_yaml(k8_config_file_path)
            return len(k8_check_config['instances']) > 0
        except Exception as err:
            log.debug("Error detecting kubernetes: %s" % str(err))
            return False

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
    DEFAULT_MASTER_PORT = 443
    DEFAULT_MASTER_NAME = 'kubernetes'  # DNS name to reach the master from a pod.
    DEFAULT_LABEL_PREFIX = 'kube_'
    DEFAULT_COLLECT_SERVICE_TAG = True
    CA_CRT_PATH = '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'
    AUTH_TOKEN_PATH = '/var/run/secrets/kubernetes.io/serviceaccount/token'

    POD_NAME_LABEL = "io.kubernetes.pod.name"
    NAMESPACE_LABEL = "io.kubernetes.pod.namespace"
    CONTAINER_NAME_LABEL = "io.kubernetes.container.name"

    def __init__(self, **kwargs):
        self.docker_util = DockerUtil()
        if 'init_config' in kwargs and 'instance' in kwargs:
            init_config = kwargs.get('init_config', {})
            instance = kwargs.get('instance', {})
        else:
            try:
                config_file_path = get_conf_path(KUBERNETES_CHECK_NAME)
                check_config = check_yaml(config_file_path)
                init_config = check_config['init_config'] or {}
                instance = check_config['instances'][0] or {}
            # kubernetes.yaml was not found
            except IOError as ex:
                log.error(ex.message)
                init_config, instance = {}, {}
            except Exception:
                log.error('Kubernetes configuration file is invalid. '
                          'Trying connecting to kubelet with default settings anyway...')
                init_config, instance = {}, {}

        self.method = instance.get('method', KubeUtil.DEFAULT_METHOD)
        self._node_ip = self._node_name = None  # lazy evaluation
        self.host_name = os.environ.get('HOSTNAME')
        self.tls_settings = self._init_tls_settings(instance)

        # apiserver
        if 'api_server_url' in instance:
            self.kubernetes_api_root_url = instance.get('api_server_url')
        else:
            master_host = os.environ.get('KUBERNETES_SERVICE_HOST') or self.DEFAULT_MASTER_NAME
            master_port = os.environ.get('KUBERNETES_SERVICE_PORT') or self.DEFAULT_MASTER_PORT
            self.kubernetes_api_root_url = 'https://%s:%s' % (master_host, master_port)

        self.kubernetes_api_url = '%s/api/v1' % self.kubernetes_api_root_url

        # Service mapping helper class
        self._service_mapper = PodServiceMapper(self)
        from config import _is_affirmative
        self.collect_service_tag = _is_affirmative(instance.get('collect_service_tags', KubeUtil.DEFAULT_COLLECT_SERVICE_TAG))


        # leader status triggers event collection
        self.is_leader = False
        self.leader_elector = None
        self.leader_lease_duration = instance.get('leader_lease_duration')

        # kubelet
        # If kubelet_api_url is None, init_kubelet didn't succeed yet.
        self.init_success = False
        self.kubelet_api_url = None
        self.init_retry_interval = init_config.get('init_retry_interval', DEFAULT_RETRY_INTERVAL)
        self.last_init_retry = None
        self.left_init_retries = init_config.get('init_retries', DEFAULT_INIT_RETRIES) + 1
        self.init_kubelet(instance)

        self.kube_label_prefix = instance.get('label_to_tag_prefix', KubeUtil.DEFAULT_LABEL_PREFIX)
        self.kube_node_labels = instance.get('node_labels_to_host_tags', {})

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

        if ('apiserver_client_cert' not in tls_settings) or ('kubelet_client_cert' not in tls_settings):
            # Only lookup token if we don't have client certs for both
            token = self.get_auth_token(instance)
            if token:
                tls_settings['bearer_token'] = token

        return tls_settings

    def init_kubelet(self, instance):
        """
        Handles the retry logic around _locate_kubelet.
        Once _locate_kubelet succeeds, initialize all kubelet-related
        URLs and settings.
        """
        if self.left_init_retries == 0:
            raise Exception("Kubernetes client initialization failed permanently. "
                "Kubernetes-related features will fail.")

        now = time.time()

        # last retry was less than retry_interval ago
        if self.last_init_retry and now <= self.last_init_retry + self.init_retry_interval:
            return
        # else it's the first try, or last retry was long enough ago
        self.last_init_retry = now
        self.left_init_retries -= 1

        try:
            self.kubelet_api_url = self._locate_kubelet(instance)
        except Exception as ex:
            log.error("Failed to initialize kubelet connection. Will retry %s time(s). Error: %s" % (self.left_init_retries, str(ex)))
            return
        if not self.kubelet_api_url:
            log.error("Failed to initialize kubelet connection. Will retry %s time(s)." % self.left_init_retries)
            return

        self.init_success = True

        self.kubelet_host = self.kubelet_api_url.split(':')[1].lstrip('/')
        self.pods_list_url = urljoin(self.kubelet_api_url, KubeUtil.PODS_LIST_PATH)
        self.kube_health_url = urljoin(self.kubelet_api_url, KubeUtil.KUBELET_HEALTH_PATH)

        # namespace of the agent pod
        try:
            self.self_namespace = self.get_self_namespace()
        except Exception:
            log.warning("Failed to get the agent pod namespace, defaulting to default.")
            self.self_namespace = DEFAULT_NAMESPACE

        # cadvisor
        self.cadvisor_port = instance.get('port', KubeUtil.DEFAULT_CADVISOR_PORT)
        self.cadvisor_url = '%s://%s:%d' % (self.method, self.kubelet_host, self.cadvisor_port)
        self.metrics_url = urljoin(self.cadvisor_url, KubeUtil.METRICS_PATH)
        self.machine_info_url = urljoin(self.cadvisor_url, KubeUtil.MACHINE_INFO_PATH)

    def _locate_kubelet(self, instance):
        """
        Kubelet may or may not accept un-authenticated http requests.
        If it doesn't we need to use its HTTPS API that may or may not
        require auth.
        Returns the kubelet URL or raises.
        """
        host = os.environ.get('KUBERNETES_KUBELET_HOST') or instance.get("host")
        if not host:
            # if no hostname was provided, use the docker hostname if cert
            # validation is not required, the kubernetes hostname otherwise.
            host = self.docker_util.get_hostname(should_resolve=True)
            if self.tls_settings.get('kubelet_verify'):
                try:
                    host = self.get_node_hostname(host)
                except Exception:
                    pass

        # check if the no-auth endpoint is enabled
        port = instance.get('kubelet_port', KubeUtil.DEFAULT_HTTP_KUBELET_PORT)
        no_auth_url = 'http://%s:%s' % (host, port)
        test_url = urljoin(no_auth_url, KubeUtil.KUBELET_HEALTH_PATH)
        try:
            self.perform_kubelet_query(test_url)
            return no_auth_url
        except Exception:
            log.debug("Couldn't query kubelet over HTTP, assuming it's not in no_auth mode.")

        port = instance.get('kubelet_port', KubeUtil.DEFAULT_HTTPS_KUBELET_PORT)
        https_url = 'https://%s:%s' % (host, port)
        test_url = urljoin(https_url, KubeUtil.KUBELET_HEALTH_PATH)
        try:
            self.perform_kubelet_query(test_url)
            return https_url
        except Exception as ex:
            log.warning("Couldn't query kubelet over HTTP, assuming it's not in no_auth mode.")
            raise ex

    def get_self_namespace(self):
        pods = self.retrieve_pods_list()
        for pod in pods.get('items', []):
            if pod.get('metadata', {}).get('name') == self.host_name:
                return pod['metadata']['namespace']
        log.warning("Couldn't find the agent pod and namespace, using the default.")
        return DEFAULT_NAMESPACE

    def get_node_hostname(self, host):
        """
        Query the API server for the kubernetes hostname of the node
        using the docker hostname as a filter.
        """
        node_filter = {'labelSelector': 'kubernetes.io/hostname=%s' % host}
        node = self.retrieve_json_auth(
            self.kubernetes_api_url + '/nodes?%s' % urlencode(node_filter)
        ).json()
        if len(node['items']) != 1:
            log.error('Error while getting node hostname: expected 1 node, got %s.' % len(node['items']))
        else:
            addresses = (node or {}).get('items', [{}])[0].get('status', {}).get('addresses', [])
            for address in addresses:
                if address.get('type') == 'Hostname':
                    return address['address']
        return None

    def get_kube_pod_tags(self, excluded_keys=None):
        """
        Gets pods' labels as tags + creator and service tags.
        Returns a dict{namespace/podname: [tags]}
        """
        if not self.init_success:
            log.warning("Kubernetes client is not initialized, can't get pod tags.")
            return {}
        pods = self.retrieve_pods_list()
        return self.extract_kube_pod_tags(pods, excluded_keys=excluded_keys)

    def extract_kube_pod_tags(self, pods_list, excluded_keys=None, label_prefix=None):
        """
        Extract labels + creator and service tags from a list of
        pods coming from the kubelet API.

        :param excluded_keys: labels to skip
        :param label_prefix: prefix for label->tag conversion, None defaults
        to the configuration option label_to_tag_prefix
        Returns a dict{namespace/podname: [tags]}
        """
        excluded_keys = excluded_keys or []
        kube_labels = defaultdict(list)
        pod_items = pods_list.get("items") or []
        label_prefix = label_prefix or self.kube_label_prefix
        for pod in pod_items:
            metadata = pod.get("metadata", {})
            name = metadata.get("name")
            namespace = metadata.get("namespace")
            labels = metadata.get("labels", {})
            if name and namespace:
                key = "%s/%s" % (namespace, name)

                # Extract creator tags
                podtags = self.get_pod_creator_tags(metadata)

                # Extract services tags
                if self.collect_service_tag:
                    for service in self.match_services_for_pod(metadata):
                        if service is not None:
                            podtags.append(u'kube_service:%s' % service)

                # Extract labels
                for k, v in labels.iteritems():
                    if k in excluded_keys:
                        continue
                    podtags.append(u"%s%s:%s" % (label_prefix, k, v))

                kube_labels[key] = podtags

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

    def get_deployment_for_replicaset(self, rs_name):
        """
        Get the deployment name for a given replicaset name
        For now, the rs name's first part always is the deployment's name, see
        https://github.com/kubernetes/kubernetes/blob/release-1.6/pkg/controller/deployment/sync.go#L299
        But it might change in a future k8s version. The other way to match RS and deployments is
        to parse and cache /apis/extensions/v1beta1/replicasets, mirroring PodServiceMapper
        """
        end = rs_name.rfind("-")
        if end > 0 and rs_name[end + 1:].isdigit():
            return rs_name[0:end]
        else:
            return None

    def perform_kubelet_query(self, url, verbose=True, timeout=10):
        """
        Perform and return a GET request against kubelet. Support auth and TLS validation.
        """
        tls_context = self.tls_settings

        headers = None
        cert = tls_context.get('kubelet_client_cert')
        verify = tls_context.get('kubelet_verify', DEFAULT_TLS_VERIFY)

        # if cert-based auth is enabled, don't use the token.
        if not cert and url.lower().startswith('https') and 'bearer_token' in self.tls_settings:
            headers = {'Authorization': 'Bearer {}'.format(self.tls_settings.get('bearer_token'))}

        return requests.get(url, timeout=timeout, verify=verify,
            cert=cert, headers=headers, params={'verbose': verbose})

    def get_apiserver_auth_settings(self):
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
        headers = {'Authorization': 'Bearer {}'.format(bearer_token)} if bearer_token else {}
        headers['content-type'] = 'application/json'
        return cert, headers, verify

    def retrieve_json_auth(self, url, params=None, timeout=3):
        cert, headers, verify = self.get_apiserver_auth_settings()
        res = requests.get(url, timeout=timeout, headers=headers, verify=verify, cert=cert, params=params)
        res.raise_for_status()
        return res

    def post_json_to_apiserver(self, url, data, timeout=3):
        cert, headers, verify = self.get_apiserver_auth_settings()
        res = requests.post(url, timeout=timeout, headers=headers, verify=verify, cert=cert, data=json.dumps(data))
        res.raise_for_status()
        return res

    def put_json_to_apiserver(self, url, data, timeout=3):
        cert, headers, verify = self.get_apiserver_auth_settings()
        res = requests.put(url, timeout=timeout, headers=headers, verify=verify, cert=cert, data=json.dumps(data))
        res.raise_for_status()
        return res

    def delete_to_apiserver(self, url, timeout=3):
        cert, headers, verify = self.get_apiserver_auth_settings()
        res = requests.delete(url, timeout=timeout, headers=headers, verify=verify, cert=cert)
        res.raise_for_status()
        return res

    def get_node_info(self):
        """
        Return the IP address and the hostname of the node where the pod is running.
        """
        if None in (self._node_ip, self._node_name):
            self._fetch_host_data()
        return self._node_ip, self._node_name

    def get_node_hosttags(self):
        tags = []

        # API server version
        try:
            request_url = "%s/version" % self.kubernetes_api_root_url
            master_info = self.retrieve_json_auth(request_url).json()
            version = master_info.get("gitVersion")
            tags.append("kube_master_version:%s" % version[1:])
        except Exception as e:
            # Intentional use of non-safe lookups to get the exception in the debug logs
            # if the parsing were to fail
            log.debug("Error getting Kube master version: %s" % str(e))

        # Kubelet version & labels
        if not self.init_success:
            log.warning("Kubelet client failed to initialize, kubelet host tags will be missing for now.")
            return tags
        try:
            _, node_name = self.get_node_info()
            if not node_name:
                raise ValueError("node name missing or empty")
            request_url = "%s/nodes/%s" % (self.kubernetes_api_url, node_name)
            node_info = self.retrieve_json_auth(request_url).json()
            version = node_info.get("status").get("nodeInfo").get("kubeletVersion")
            tags.append("kubelet_version:%s" % version[1:])

            node_labels = node_info.get('metadata', {}).get('labels', {})
            for l_name, t_name in self.kube_node_labels.iteritems():
                if l_name in node_labels:
                    tags.append('%s:%s' % (t_name, node_labels[l_name]))

        except Exception as e:
            log.debug("Error getting Kubelet version: %s" % str(e))

        return tags

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
    def get_auth_token(cls, instance):
        """
        Return a string containing the authorization token for the pod.
        """

        token_path = instance.get('bearer_token_path', cls.AUTH_TOKEN_PATH)
        try:
            with open(token_path) as f:
                return f.read().strip()
        except IOError as e:
            log.error('Unable to read token from {}: {}'.format(token_path, e))

        return None

    def match_services_for_pod(self, pod_metadata, refresh=False):
        """
        Match the pods labels with services' label selectors to determine the list
        of services that point to that pod. Returns an array of service names.

        Pass refresh=True if you want to bypass the cached cid->services mapping (after a service change)
        """
        s = self._service_mapper.match_services_for_pod(pod_metadata, refresh, names=True)
        #log.warning("Matches for %s: %s" % (pod_metadata.get('name'), str(s)))
        return s

    def get_event_retriever(self, namespaces=None, kinds=None, delay=None):
        """
        Returns a KubeEventRetriever object ready for action
        """
        return KubeEventRetriever(self, namespaces, kinds, delay)

    def match_containers_for_pods(self, pod_uids, podlist=None):
        """
        Reads a set of pod uids and returns the set of docker
        container ids they manage
        podlist should be a recent self.retrieve_pods_list return value,
        if not given that method will be called
        """
        cids = set()

        if not isinstance(pod_uids, set) or len(pod_uids) < 1:
            return cids

        if podlist is None:
            podlist = self.retrieve_pods_list()

        for pod in podlist.get('items', {}):
            uid = pod.get('metadata', {}).get('uid', None)
            if uid in pod_uids:
                for container in pod.get('status', {}).get('containerStatuses', None):
                    id = container.get('containerID', "")
                    if id.startswith("docker://"):
                        cids.add(id[9:])

        return cids

    def get_pod_creator(self, pod_metadata):
        """
        Get the pod's creator from its metadata and returns a
        tuple (creator_kind, creator_name)

        This allows for consitency across code path
        """
        try:
            created_by = json.loads(pod_metadata['annotations']['kubernetes.io/created-by'])
            creator_kind = created_by.get('reference', {}).get('kind')
            creator_name = created_by.get('reference', {}).get('name')
            return (creator_kind, creator_name)
        except Exception:
            log.debug('Could not parse creator for pod ' + pod_metadata.get('name', ''))
            return (None, None)

    def get_pod_creator_tags(self, pod_metadata, legacy_rep_controller_tag=False):
        """
        Get the pod's creator from its metadata and returns a list of tags
        in the form kube_$kind:$name, ready to add to the metrics
        """
        try:
            tags = []
            creator_kind, creator_name = self.get_pod_creator(pod_metadata)
            if creator_kind in CREATOR_KIND_TO_TAG and creator_name:
                tags.append("%s:%s" % (CREATOR_KIND_TO_TAG[creator_kind], creator_name))
                if creator_kind == 'ReplicaSet':
                    deployment = self.get_deployment_for_replicaset(creator_name)
                    if deployment:
                        tags.append("%s:%s" % (CREATOR_KIND_TO_TAG['Deployment'], deployment))
            if legacy_rep_controller_tag and creator_kind != 'ReplicationController' and creator_name:
                tags.append('kube_replication_controller:{0}'.format(creator_name))

            return tags
        except Exception:
            log.warning('Could not parse creator tags for pod ' + pod_metadata.get('name'))
            return []

    def process_events(self, event_array, podlist=None):
        """
        Reads a list of kube events, invalidates caches and and computes a set
        of containers impacted by the changes, to refresh service discovery
        Pod creation/deletion events are ignored for now, as docker_daemon already
        sends container creation/deletion events to SD

        Pod->containers matching is done using match_containers_for_pods
        """
        try:
            pods = set()
            if self._service_mapper:
                pods.update(self._service_mapper.process_events(event_array))
            return self.match_containers_for_pods(pods, podlist)
        except Exception as e:
            log.warning("Error processing events %s: %s" % (str(event_array), e))
            return set()

    def refresh_leader(self):
        if not self.init_success:
            log.warning("Kubelet client is not initialized, leader election is disabled.")
            return
        if not self.leader_elector:
            self.leader_elector = LeaderElector(self)
        self.leader_elector.try_acquire_or_refresh()

    def image_name_resolver(self, image):
        """
        Wraps around the sibling dockerutil method and catches exceptions
        """
        if image is None:
            return None
        try:
            return self.docker_util.image_name_resolver(image)
        except Exception as e:
            log.warning("Error resolving image name: %s", str(e))
            return image
