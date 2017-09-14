# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# std
import logging
import simplejson as json

# 3rd party
from docker.errors import NullResource, NotFound

# project
from utils.dockerutil import (
    DockerUtil,
    SWARM_SVC_LABEL,
    RANCHER_CONTAINER_IP,
    RANCHER_CONTAINER_NAME,
    RANCHER_SVC_NAME,
    RANCHER_STACK_NAME
)
from utils.kubernetes import KubeUtil
from utils.platform import Platform
from utils.service_discovery.abstract_sd_backend import AbstractSDBackend
from utils.service_discovery.config_stores import get_config_store
from utils.orchestrator import MetadataCollector

DATADOG_ID = 'com.datadoghq.sd.check.id'

log = logging.getLogger(__name__)


class _SDDockerBackendConfigFetchState(object):
    def __init__(self, inspect_fn, kube_pods=None):
        self.inspect_cache = {}

        self.inspect_fn = inspect_fn
        self.kube_pods = kube_pods

    def inspect_container(self, c_id):
        if c_id in self.inspect_cache:
            return self.inspect_cache[c_id]

        try:
            self.inspect_cache[c_id] = inspect = self.inspect_fn(c_id)
        except (NullResource, NotFound):
            self.inspect_cache[c_id] = inspect = {}

        return inspect

    def get_kube_container_status(self, c_id):
        for pod in self.kube_pods:
            c_statuses = pod.get('status', {}).get('containerStatuses', [])
            for status in c_statuses:
                if c_id == status.get('containerID', '').split('//')[-1]:
                    return status
        return {}

    def get_kube_container_name(self, c_id):
        return self.get_kube_container_status(c_id).get('name')

    def get_kube_container_spec(self, c_id):
        c_name = self.get_kube_container_name(c_id)
        containers = self.get_kube_config(c_id, 'spec').get('containers', [])
        for co in containers:
            if co.get('name') == c_name:
                return co
        return None

    def get_kube_config(self, c_id, key):
        """Get a part of a pod config from the kubernetes API"""
        for pod in self.kube_pods:
            c_statuses = pod.get('status', {}).get('containerStatuses', [])
            for status in c_statuses:
                if c_id == status.get('containerID', '').split('//')[-1]:
                    return pod.get(key, {})
        return {}


class SDDockerBackend(AbstractSDBackend):
    """Docker-based service discovery"""

    def __init__(self, agentConfig):
        try:
            self.config_store = get_config_store(agentConfig=agentConfig)
        except Exception as e:
            log.error('Failed to instantiate the config store client. '
                      'Auto-config only will be used. %s' % str(e))
            agentConfig['sd_config_backend'] = None
            self.config_store = get_config_store(agentConfig=agentConfig)

        self.dockerutil = DockerUtil(config_store=self.config_store)
        self.kubeutil = None
        if Platform.is_k8s():
            try:
                self.kubeutil = KubeUtil()
            except Exception as ex:
                log.error("Couldn't instantiate the kubernetes client, "
                    "subsequent kubernetes calls will fail as well. Error: %s" % str(ex))

        self.metadata_collector = MetadataCollector()

        self.VAR_MAPPING = {
            'host': self._get_host_address,
            'pid': self._get_container_pid,
            'port': self._get_port,
            'container-name': self._get_container_name,
            'tags': self._get_additional_tags,
        }

        AbstractSDBackend.__init__(self, agentConfig)

    def _make_fetch_state(self):
        pod_list = []
        if Platform.is_k8s():
            if not self.kubeutil or not self.kubeutil.init_success:
                log.error("kubelet client not initialized, cannot retrieve pod list.")
            else:
                try:
                    pod_list = self.kubeutil.retrieve_pods_list().get('items', [])
                except Exception as ex:
                    log.warning("Failed to retrieve pod list: %s" % str(ex))
        return _SDDockerBackendConfigFetchState(self.dockerutil.client.inspect_container, pod_list)

    def update_checks(self, changed_containers):
        """
        Takes a list of container IDs that changed recently
        and marks their corresponding checks as
        """
        if not self.dockerutil.client:
            log.warning("Docker client is not initialized, pausing auto discovery.")
            return

        state = self._make_fetch_state()

        conf_reload_set = set()
        for c_id in changed_containers:
            checks = self._get_checks_to_refresh(state, c_id)
            if checks:
                conf_reload_set.update(set(checks))

        if conf_reload_set:
            self.reload_check_configs = conf_reload_set

    def _get_checks_to_refresh(self, state, c_id):
        """Get the list of checks applied to a container from the identifier_to_checks cache in the config store.
        Use the DATADOG_ID label or the image."""
        inspect = state.inspect_container(c_id)

        # If the container was removed we can't tell which check is concerned
        # so we have to reload everything.
        # Same thing if it's stopped and we're on Kubernetes in auto_conf mode
        # because the pod was deleted and its template could have been in the annotations.
        if not inspect or \
                (not inspect.get('State', {}).get('Running')
                    and Platform.is_k8s() and not self.agentConfig.get('sd_config_backend')):
            self.reload_check_configs = True
            return

        labels = inspect.get('Config', {}).get('Labels', {})
        identifier = labels.get(DATADOG_ID) or \
            self.dockerutil.image_name_extractor(inspect)

        platform_kwargs = {}
        if Platform.is_k8s():
            kube_metadata = state.get_kube_config(c_id, 'metadata') or {}
            platform_kwargs = {
                'kube_annotations': kube_metadata.get('annotations'),
                'kube_container_name': state.get_kube_container_name(c_id),
            }
        if labels:
            platform_kwargs['docker_labels'] = labels
        return self.config_store.get_checks_to_refresh(identifier, **platform_kwargs)

    def _get_container_pid(self, state, cid, tpl_var):
        """Extract the host-namespace pid of the container pid 0"""
        pid = state.inspect_container(cid).get('State', {}).get('Pid')
        if not pid:
            return None

        return str(pid)

    def _get_host_address(self, state, c_id, tpl_var):
        """Extract the container IP from a docker inspect object, or the kubelet API."""
        c_inspect = state.inspect_container(c_id)
        c_id = c_inspect.get('Id', '')
        c_img = self.dockerutil.image_name_extractor(c_inspect)

        networks = c_inspect.get('NetworkSettings', {}).get('Networks') or {}
        ip_dict = {}
        for net_name, net_desc in networks.iteritems():
            ip = net_desc.get('IPAddress')
            if ip:
                ip_dict[net_name] = ip
        ip_addr = self._extract_ip_from_networks(ip_dict, tpl_var)
        if ip_addr:
            return ip_addr

        # try to get the bridge (default) IP address
        log.debug("No IP address was found in container %s (%s) "
            "networks, trying with the IPAddress field" % (c_id[:12], c_img))
        ip_addr = c_inspect.get('NetworkSettings', {}).get('IPAddress')
        if ip_addr:
            return ip_addr

        if Platform.is_k8s():
            # kubernetes case
            log.debug("Couldn't find the IP address for container %s (%s), "
                      "using the kubernetes way." % (c_id[:12], c_img))
            pod_ip = state.get_kube_config(c_id, 'status').get('podIP')
            if pod_ip:
                return pod_ip

        if Platform.is_rancher():
            # try to get the rancher IP address
            log.debug("No IP address was found in container %s (%s) "
                "trying with the Rancher label" % (c_id[:12], c_img))

            ip_addr = c_inspect.get('Config', {}).get('Labels', {}).get(RANCHER_CONTAINER_IP)
            if ip_addr:
                return ip_addr.split('/')[0]

        log.error("No IP address was found for container %s (%s)" % (c_id[:12], c_img))
        return None

    def _extract_ip_from_networks(self, ip_dict, tpl_var):
        """Extract a single IP from a dictionary made of network names and IPs."""
        if not ip_dict:
            return None
        tpl_parts = tpl_var.split('_', 1)

        # no specifier
        if len(tpl_parts) < 2:
            log.debug("No key was passed for template variable %s." % tpl_var)
            return self._get_fallback_ip(ip_dict)
        else:
            res = ip_dict.get(tpl_parts[-1])
            if res is None:
                log.warning("The key passed for template variable %s was not found." % tpl_var)
                return self._get_fallback_ip(ip_dict)
            else:
                return res

    def _get_fallback_ip(self, ip_dict):
        """try to pick the bridge key, falls back to the value of the last key"""
        if 'bridge' in ip_dict:
            log.debug("Using the bridge network.")
            return ip_dict['bridge']
        else:
            last_key = sorted(ip_dict.iterkeys())[-1]
            log.debug("Trying with the last (sorted) network: '%s'." % last_key)
            return ip_dict[last_key]

    def _get_port(self, state, c_id, tpl_var):
        """Extract a port from a container_inspect or the k8s API given a template variable."""
        container_inspect = state.inspect_container(c_id)

        try:
            ports = map(lambda x: x.split('/')[0], container_inspect['NetworkSettings']['Ports'].keys())
            if len(ports) == 0: # There might be a key Port in NetworkSettings but no ports so we raise IndexError to check in ExposedPorts
                raise IndexError
        except (IndexError, KeyError, AttributeError):
            # try to get ports from the docker API. Works if the image has an EXPOSE instruction
            ports = map(lambda x: x.split('/')[0], container_inspect['Config'].get('ExposedPorts', {}).keys())

            # if it failed, try with the kubernetes API
            if not ports and Platform.is_k8s():
                log.debug("Didn't find the port for container %s (%s), trying the kubernetes way." %
                          (c_id[:12], container_inspect.get('Config', {}).get('Image', '')))
                spec = state.get_kube_container_spec(c_id)
                if spec:
                    ports = [str(x.get('containerPort')) for x in spec.get('ports', [])]
        ports = sorted(ports, key=int)
        return self._extract_port_from_list(ports, tpl_var)

    def _extract_port_from_list(self, ports, tpl_var):
        if not ports:
            return None

        tpl_parts = tpl_var.split('_', 1)

        if len(tpl_parts) == 1:
            log.debug("No index was passed for template variable %s. "
                      "Trying with the last element." % tpl_var)
            return ports[-1]

        try:
            idx = tpl_parts[-1]
            return ports[int(idx)]
        except ValueError:
            log.error("Port index is not an integer. Using the last element instead.")
        except IndexError:
            log.error("Port index is out of range. Using the last element instead.")
        return ports[-1]

    def get_tags(self, state, c_id):
        """Extract useful tags from docker or platform APIs. These are collected by default."""
        c_inspect = state.inspect_container(c_id)
        tags = self.dockerutil.extract_container_tags(c_inspect)

        if Platform.is_k8s():
            if not self.kubeutil.init_success:
                log.warning("kubelet client not initialized, kubernetes tags will be missing.")
                return tags

            pod_metadata = state.get_kube_config(c_id, 'metadata')

            if pod_metadata is None:
                log.warning("Failed to fetch pod metadata for container %s."
                            " Kubernetes tags will be missing." % c_id[:12])
                return tags

            # get pod labels
            kube_labels = pod_metadata.get('labels', {})
            for label, value in kube_labels.iteritems():
                tags.append('%s:%s' % (label, value))

            # get kubernetes namespace
            namespace = pod_metadata.get('namespace')
            tags.append('kube_namespace:%s' % namespace)

            if not self.kubeutil:
                log.warning("The agent can't connect to kubelet, creator and "
                    "service tags will be missing for container %s." % c_id[:12])
            else:
                # add creator tags
                creator_tags = self.kubeutil.get_pod_creator_tags(pod_metadata)
                tags.extend(creator_tags)

                # add services tags
                if self.kubeutil.collect_service_tag:
                    services = self.kubeutil.match_services_for_pod(pod_metadata)
                    for s in services:
                        if s is not None:
                            tags.append('kube_service:%s' % s)

        elif Platform.is_swarm():
            c_labels = c_inspect.get('Config', {}).get('Labels', {})
            swarm_svc = c_labels.get(SWARM_SVC_LABEL)
            if swarm_svc:
                tags.append('swarm_service:%s' % swarm_svc)

        elif Platform.is_rancher():
            service_name = c_inspect.get('Config', {}).get('Labels', {}).get(RANCHER_SVC_NAME)
            stack_name = c_inspect.get('Config', {}).get('Labels', {}).get(RANCHER_STACK_NAME)
            container_name = c_inspect.get('Config', {}).get('Labels', {}).get(RANCHER_CONTAINER_NAME)
            if service_name:
                tags.append('rancher_service:%s' % service_name)
            if stack_name:
                tags.append('rancher_stack:%s' % stack_name)
            if container_name:
                tags.append('rancher_container:%s' % container_name)

        if self.metadata_collector.has_detected():
            orch_tags = self.metadata_collector.get_container_tags(co=c_inspect)
            tags.extend(orch_tags)

        return tags

    def _get_container_name(self, state, c_id, tpl_var):
        container_inspect = state.inspect_container(c_id)
        return container_inspect.get('Name', '').lstrip('/')

    def _get_additional_tags(self, state, c_id, *args):
        tags = []

        if Platform.is_k8s():
            pod_metadata = state.get_kube_config(c_id, 'metadata')
            pod_spec = state.get_kube_config(c_id, 'spec')
            if pod_metadata is None or pod_spec is None:
                log.warning("Failed to fetch pod metadata or pod spec for container %s."
                            " Additional Kubernetes tags may be missing." % c_id[:12])
                return []
            tags.append('node_name:%s' % pod_spec.get('nodeName'))
            tags.append('pod_name:%s' % pod_metadata.get('name'))

            c_inspect = state.inspect_container(c_id)
            c_name = c_inspect.get('Config', {}).get('Labels', {}).get(KubeUtil.CONTAINER_NAME_LABEL)
            if c_name:
                tags.append('kube_container_name:%s' % c_name)
        return tags

    def get_configs(self):
        """Get the config for all docker containers running on the host."""
        configs = {}
        if not self.dockerutil.client:
            log.warning("Docker client is not initialized, pausing auto discovery.")
            return configs

        state = self._make_fetch_state()
        containers = [(
            self.dockerutil.image_name_extractor(container),
            container.get('Id'), container.get('Labels')
        ) for container in self.dockerutil.client.containers()]

        for image, cid, labels in containers:
            try:
                # value of the DATADOG_ID tag or the image name if the label is missing
                identifier = self.get_config_id(image, labels)
                check_configs = self._get_check_configs(state, cid, identifier, labels) or []
                for conf in check_configs:
                    source, (check_name, init_config, instance) = conf

                    # build instances list if needed
                    if configs.get(check_name) is None:
                        if isinstance(instance, list):
                            configs[check_name] = (source, (init_config, instance))
                        else:
                            configs[check_name] = (source, (init_config, [instance]))
                    else:
                        conflict_init_msg = 'Different versions of `init_config` found for check {}. ' \
                            'Keeping the first one found.'
                        if configs[check_name][1][0] != init_config:
                            log.warning(conflict_init_msg.format(check_name))
                        if isinstance(instance, list):
                            for inst in instance:
                                configs[check_name][1][1].append(inst)
                        else:
                            configs[check_name][1][1].append(instance)
            except Exception:
                log.exception('Building config for container %s based on image %s using service '
                              'discovery failed, leaving it alone.' % (cid[:12], image))
        return configs

    def get_config_id(self, image, labels):
        """Look for a DATADOG_ID label, return its value or the image name if missing"""
        return labels.get(DATADOG_ID) or image

    def _get_check_configs(self, state, c_id, identifier, labels=None):
        """Retrieve configuration templates and fill them with data pulled from docker and tags."""
        platform_kwargs = {}
        if Platform.is_k8s():
            kube_metadata = state.get_kube_config(c_id, 'metadata') or {}
            platform_kwargs = {
                'kube_container_name': state.get_kube_container_name(c_id),
                'kube_annotations': kube_metadata.get('annotations'),
            }
        if labels:
            platform_kwargs['docker_labels'] = labels

        config_templates = self._get_config_templates(identifier, **platform_kwargs)
        if not config_templates:
            return None

        check_configs = []
        tags = self.get_tags(state, c_id)
        for config_tpl in config_templates:
            source, config_tpl = config_tpl
            check_name, init_config_tpl, instance_tpl, variables = config_tpl

            # covering mono-instance and multi-instances cases
            tmpl_array = instance_tpl
            if not isinstance(instance_tpl, list):
                tmpl_array = [instance_tpl]

            # insert tags in instance_tpl and process values for template variables
            result_instances = []
            result_init_config = None
            for inst_tmpl in tmpl_array:
                instance_tpl, var_values = self._fill_tpl(state, c_id, inst_tmpl, variables, tags)
                tpl = self._render_template(init_config_tpl or {}, instance_tpl or {}, var_values)
                if tpl and len(tpl) == 2:
                    init_config, instance = tpl
                    result_instances.append(instance)
                    if not result_init_config:
                        result_init_config = init_config
                    elif result_init_config != init_config:
                        self.log.warning("Different versions of `init_config` found for "
                            "check {}. Keeping the first one found.".format('check_name'))
            check_configs.append((source, (check_name, result_init_config, result_instances)))

        return check_configs

    def _get_config_templates(self, identifier, **platform_kwargs):
        """Extract config templates for an identifier from a K/V store and returns it as a dict object."""
        config_backend = self.agentConfig.get('sd_config_backend')
        templates = []
        if config_backend is None:
            auto_conf = True
        else:
            auto_conf = False

        # format [(source, ('ident', {init_tpl}, {instance_tpl}))]
        raw_tpls = self.config_store.get_check_tpls(identifier, auto_conf=auto_conf, **platform_kwargs)
        for tpl in raw_tpls:
            # each template can come from either auto configuration or user-supplied templates
            try:
                source, (check_name, init_config_tpl, instance_tpl) = tpl
            except (TypeError, IndexError, ValueError):
                log.debug('No template was found for identifier %s, leaving it alone: %s' % (identifier, tpl))
                return None
            try:
                # build a list of all variables to replace in the template
                variables = self.PLACEHOLDER_REGEX.findall(str(init_config_tpl)) + \
                    self.PLACEHOLDER_REGEX.findall(str(instance_tpl))
                variables = map(lambda x: x.strip('%'), variables)
                if not isinstance(init_config_tpl, dict):
                    init_config_tpl = json.loads(init_config_tpl or '{}')
                if not isinstance(instance_tpl, dict) and not isinstance(instance_tpl, list):
                    instance_tpl = json.loads(instance_tpl or '{}')
            except json.JSONDecodeError:
                log.exception('Failed to decode the JSON template fetched for check {0}. Its configuration'
                              ' by service discovery failed for ident  {1}.'.format(check_name, identifier))
                return None

            templates.append((source,
                              (check_name, init_config_tpl, instance_tpl, variables)))

        return templates

    def _fill_tpl(self, state, c_id, instance_tpl, variables, c_tags=None):
        """Add container tags to instance templates and build a
           dict from template variable names and their values."""
        var_values = {}
        c_image = state.inspect_container(c_id).get('Config', {}).get('Image', '')

        # add only default c_tags to the instance to avoid duplicate tags from conf
        if c_tags:
            tags = c_tags[:] # shallow copy of the c_tags array
        else:
            tags = []
        if tags:
            tpl_tags = instance_tpl.get('tags', [])
            if isinstance(tpl_tags, dict):
                for key, val in tpl_tags.iteritems():
                    tags.append("{}:{}".format(key, val))
            else:
                tags += tpl_tags if isinstance(tpl_tags, list) else [tpl_tags]
            instance_tpl['tags'] = list(set(tags))

        for var in variables:
            # variables can be suffixed with an index in case several values are found
            if var.split('_')[0] in self.VAR_MAPPING:
                try:
                    res = self.VAR_MAPPING[var.split('_')[0]](state, c_id, var)
                    if res is None:
                        raise ValueError("Invalid value for variable %s." % var)
                    var_values[var] = res
                except Exception as ex:
                    log.error("Could not find a value for the template variable %s for container %s "
                              "(%s): %s" % (var, c_id[:12], c_image, str(ex)))
            else:
                log.error("No method was found to interpolate template variable %s for container %s "
                          "(%s)." % (var, c_id[:12], c_image))

        return instance_tpl, var_values
