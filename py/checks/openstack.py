# stdlib
from datetime import datetime, timedelta
from urlparse import urljoin

# project
from checks import AgentCheck
from util import get_hostname

# 3p
import requests
import simplejson as json

SOURCE_TYPE = 'openstack'

DEFAULT_KEYSTONE_API_VERSION = 'v3'
DEFAULT_NOVA_API_VERSION = 'v2.1'
DEFAULT_NEUTRON_API_VERSION = 'v2.0'

DEFAULT_API_REQUEST_TIMEOUT = 5 # seconds

NOVA_HYPERVISOR_METRICS = [
    'current_workload',
    'disk_available_least',
    'free_disk_gb',
    'free_ram_mb',
    'local_gb',
    'local_gb_used',
    'memory_mb',
    'memory_mb_used',
    'running_vms',
    'vcpus',
    'vcpus_used',
]

NOVA_SERVER_METRICS = [
    "hdd_errors",
    "hdd_read",
    "hdd_read_req",
    "hdd_write",
    "hdd_write_req",
    "memory",
    "memory-actual",
    "memory-rss",
    "cpu0_time",

    "vda_errors",
    "vda_read",
    "vda_read_req",
    "vda_write",
    "vda_write_req"
]

NOVA_SERVER_INTERFACE_SEGMENTS = ['_rx', '_tx']

PROJECT_METRICS = dict([
    ("maxImageMeta", "max_image_meta"),
    ("maxPersonality", "max_personality"),
    ("maxPersonalitySize", "max_personality_size"),
    ("maxSecurityGroupRules", "max_security_group_rules"),
    ("maxSecurityGroups", "max_security_groups"),
    ("maxServerMeta", "max_server_meta"),
    ("maxTotalCores", "max_total_cores"),
    ("maxTotalFloatingIps", "max_total_floating_ips"),
    ("maxTotalInstances", "max_total_instances"),
    ("maxTotalKeypairs", "max_total_keypairs"),
    ("maxTotalRAMSize", "max_total_ram_size"),

    ("totalImageMetaUsed", "total_image_meta_used"),
    ("totalPersonalityUsed", "total_personality_used"),
    ("totalPersonalitySizeUsed", "total_personality_size_used"),
    ("totalSecurityGroupRulesUsed", "total_security_group_rules_used"),
    ("totalSecurityGroupsUsed", "total_security_groups_used"),
    ("totalServerMetaUsed", "total_server_meta_used"),
    ("totalCoresUsed", "total_cores_used"),
    ("totalFloatingIpsUsed", "total_floating_ips_used"),
    ("totalInstancesUsed", "total_instances_used"),
    ("totalKeypairsUsed", "total_keypairs_used"),
    ("totalRAMUsed", "total_ram_used"),
])

class OpenStackAuthFailure(Exception):
    pass

class InstancePowerOffFailure(Exception):
    pass

class IncompleteConfig(Exception):
    pass

class IncompleteAuthScope(IncompleteConfig):
    pass

class IncompleteIdentity(IncompleteConfig):
    pass

class MissingEndpoint(Exception):
    pass

class MissingNovaEndpoint(MissingEndpoint):
    pass

class MissingNeutronEndpoint(MissingEndpoint):
    pass

class KeystoneUnreachable(Exception):
    pass


class OpenStackProjectScope(object):
    """
    Container class for a single project's authorization scope
    Embeds the auth token to be included with API requests, and refreshes
    the token on expiry
    """
    def __init__(self, auth_token, auth_scope, service_catalog):
        self.auth_token = auth_token

        # Store some identifiers for this project
        self.project_name = auth_scope["project"].get("name")
        self.domain_id = auth_scope["project"].get("domain", {}).get("id")
        self.tenant_id = auth_scope["project"].get("id")
        self.service_catalog = service_catalog

    @classmethod
    def from_config(cls, init_config, instance_config):
        keystone_server_url = init_config.get("keystone_server_url")
        if not keystone_server_url:
            raise IncompleteConfig()

        ssl_verify = init_config.get("ssl_verify", False)
        nova_api_version = init_config.get("nova_api_version", DEFAULT_NOVA_API_VERSION)

        auth_scope = cls.get_auth_scope(instance_config)
        identity = cls.get_user_identity(instance_config)

        try:
            auth_resp = cls.request_auth_token(auth_scope, identity, keystone_server_url, ssl_verify)
        except (requests.exceptions.HTTPError, requests.exceptions.Timeout, requests.exceptions.ConnectionError):
            raise KeystoneUnreachable()

        auth_token = auth_resp.headers.get('X-Subject-Token')

        service_catalog = KeystoneCatalog.from_auth_response(
            auth_resp.json(), nova_api_version
        )

        # (NOTE): aaditya
        # In some cases, the nova url is returned without the tenant id suffixed
        # e.g. http://172.0.0.1:8774 rather than http://172.0.0.1:8774/<tenant_id>
        # It is still unclear when this happens, but for now the user can configure
        # `append_tenant_id` to manually add this suffix for downstream requests
        if instance_config.get("append_tenant_id", False):
            t_id = auth_scope["project"].get("id")

            assert t_id and t_id not in service_catalog.nova_endpoint,\
                """Incorrect use of append_tenant_id, please inspect the service catalog response of your Identity server.
                   You may need to disable this flag if your Nova service url contains the tenant_id already"""

            service_catalog.nova_endpoint = urljoin(service_catalog.nova_endpoint, t_id)

        return cls(auth_token, auth_scope, service_catalog)

    @classmethod
    def get_auth_scope(cls, instance_config):
        """
        Parse authorization scope out of init_config

        To guarantee a uniquely identifiable scope, expects either:
        {'project': {'name': 'my_project', 'domain': {'id': 'my_domain_id'}}}
        OR
        {'project': {'id': 'my_project_id'}}
        """
        auth_scope = instance_config.get('auth_scope')
        if not auth_scope or not auth_scope.get('project'):
            raise IncompleteAuthScope()

        if auth_scope['project'].get('name'):
            # We need to add a domain scope to avoid name clashes. Search for one. If not raise IncompleteConfig
            if not auth_scope['project'].get('domain', {}).get('id'):
                raise IncompleteAuthScope()
        else:
            # Assume a unique project id has been given
            if not auth_scope['project'].get('id'):
                raise IncompleteAuthScope()

        return auth_scope

    @classmethod
    def get_user_identity(cls, instance_config):
        """
        Parse user identity out of init_config

        To guarantee a uniquely identifiable user, expects
        {"user": {"name": "my_username", "password": "my_password",
                  "domain": {"id": "my_domain_id"}
                  }
        }
        """
        user = instance_config.get('user')
        if not user\
                or not user.get('name')\
                or not user.get('password')\
                or not user.get("domain")\
                or not user.get("domain").get("id"):

            raise IncompleteIdentity()

        identity = {
            "methods": ['password'],
            "password": {"user": user}
        }
        return identity

    @classmethod
    def request_auth_token(cls, auth_scope, identity, keystone_server_url, ssl_verify):
        payload = {"auth": {"scope": auth_scope, "identity": identity}}
        auth_url = urljoin(keystone_server_url, "{0}/auth/tokens".format(DEFAULT_KEYSTONE_API_VERSION))
        headers = {'Content-Type': 'application/json'}

        resp = requests.post(auth_url, headers=headers, data=json.dumps(payload), verify=ssl_verify, timeout=DEFAULT_API_REQUEST_TIMEOUT)
        resp.raise_for_status()

        return resp


class KeystoneCatalog(object):
    """
    A registry of services, scoped to the project, returned by the identity server
    Contains parsers for retrieving service endpoints from the server auth response
    """
    def __init__(self, nova_endpoint, neutron_endpoint):
        self.nova_endpoint = nova_endpoint
        self.neutron_endpoint = neutron_endpoint

    @classmethod
    def from_auth_response(cls, json_response, nova_api_version):
        return cls(
            nova_endpoint=cls.get_nova_endpoint(json_response, nova_api_version),
            neutron_endpoint=cls.get_neutron_endpoint(json_response)
        )

    @classmethod
    def get_neutron_endpoint(cls, json_resp):
        """
        Parse the service catalog returned by the Identity API for an endpoint matching the Neutron service
        Sends a CRITICAL service check when none are found registered in the Catalog
        """
        catalog = json_resp.get('token', {}).get('catalog', [])
        match = 'neutron'

        neutron_endpoint = None
        for entry in catalog:
            if entry['name'] == match:
                valid_endpoints = {}
                for ep in entry['endpoints']:
                    interface = ep.get('interface','')
                    if interface in ['public', 'internal']:
                        valid_endpoints[interface] = ep['url']

                if valid_endpoints:
                    # Favor public endpoints over internal
                    neutron_endpoint = valid_endpoints.get("public",
                                        valid_endpoints.get("internal"))
                    break
        else:
            raise MissingNeutronEndpoint()

        return neutron_endpoint

    @classmethod
    def get_nova_endpoint(cls, json_resp, nova_api_version=None):
        """
        Parse the service catalog returned by the Identity API for an endpoint matching the Nova service with the requested version
        Sends a CRITICAL service check when no viable candidates are found in the Catalog
        """
        nova_version = nova_api_version or DEFAULT_NOVA_API_VERSION
        catalog = json_resp.get('token', {}).get('catalog', [])

        nova_match = 'novav21' if nova_version == 'v2.1' else 'nova'

        for entry in catalog:
            if entry['name'] == nova_match:
                # Collect any endpoints on the public or internal interface
                valid_endpoints = {}
                for ep in entry['endpoints']:
                    interface = ep.get('interface','')
                    if interface in ['public', 'internal']:
                        valid_endpoints[interface] = ep['url']

                if valid_endpoints:
                    # Favor public endpoints over internal
                    nova_endpoint = valid_endpoints.get("public",
                                        valid_endpoints.get("internal"))
                    return nova_endpoint
        else:
            raise MissingNovaEndpoint()


class OpenStackCheck(AgentCheck):
    CACHE_TTL = {
        "aggregates": 300, # seconds
        "physical_hosts": 300,
        "hypervisors": 300
    }

    FETCH_TIME_ACCESSORS = {
        "aggregates": "_last_aggregate_fetch_time",
        "physical_hosts": "_last_host_fetch_time",
        "hypervisors": "_last_hypervisor_fetch_time"

    }

    HYPERVISOR_STATE_UP = 'up'
    HYPERVISOR_STATE_DOWN = 'down'
    NETWORK_STATE_UP = 'UP'

    NETWORK_API_SC = 'openstack.neutron.api.up'
    COMPUTE_API_SC = 'openstack.nova.api.up'
    IDENTITY_API_SC = 'openstack.keystone.api.up'

    # Service checks for individual hypervisors and networks
    HYPERVISOR_SC = 'openstack.nova.hypervisor.up'
    NETWORK_SC = 'openstack.neutron.network.up'


    HYPERVISOR_CACHE_EXPIRY = 120 # seconds

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)

        self._ssl_verify = init_config.get("ssl_verify", True)
        self.keystone_server_url = init_config.get("keystone_server_url")
        if not self.keystone_server_url:
            raise IncompleteConfig()

        ### Cache some things between runs for values that change rarely
        self._aggregate_list = None

        # Mapping of check instances to associated OpenStack project scopes
        self.instance_map = {}

        # Mapping of Nova-managed servers to tags
        self.external_host_tags = {}

    def _make_request_with_auth_fallback(self, url, headers=None, verify=True, params=None):
        """
        Generic request handler for OpenStack API requests
        Raises specialized Exceptions for commonly encountered error codes
        """
        try:
            resp = requests.get(url, headers=headers, verify=verify, params=params, timeout=DEFAULT_API_REQUEST_TIMEOUT)
            resp.raise_for_status()
        except requests.exceptions.HTTPError:
            if resp.status_code == 401:
                self.log.info('Need to reauthenticate before next check')

                # Delete the scope, we'll populate a new one on the next run for this instance
                self.delete_current_scope()
            elif resp.status_code == 409:
                raise InstancePowerOffFailure()
            else:
                raise

        return resp.json()

    def _instance_key(self, instance):
        i_key = instance.get('name')
        if not i_key:
            # We need a name to identify this instance
            raise IncompleteConfig()
        return i_key

    def delete_current_scope(self):
        scope_to_delete = self._current_scope
        for i_key, scope in self.instance_map.items():
            if scope is scope_to_delete:
                self.log.debug("Deleting current scope: %s", i_key)
                del self.instance_map[i_key]

    def get_scope_for_instance(self, instance):
        i_key = self._instance_key(instance)
        self.log.debug("Getting scope for instance %s", i_key)
        return self.instance_map[i_key]

    def set_scope_for_instance(self, instance, scope):
        i_key = self._instance_key(instance)
        self.log.debug("Setting scope for instance %s", i_key)
        self.instance_map[i_key] = scope

    def delete_scope_for_instance(self, instance):
        i_key = self._instance_key(instance)
        self.log.debug("Deleting scope for instance %s", i_key)
        del self.instance_map[i_key]

    def get_auth_token(self, instance=None):
        if not instance:
            # Assume instance scope is populated on self
            return self._current_scope.auth_token

        return self.get_scope_for_instance(instance).auth_token

    ### Network
    def get_neutron_endpoint(self, instance=None):
        if not instance:
            # Assume instance scope is populated on self
            return self._current_scope.service_catalog.neutron_endpoint

        return self.get_scope_for_instance(instance).service_catalog.neutron_endpoint

    def get_network_stats(self):
        """
        Collect stats for all reachable networks
        """

        # FIXME: (aaditya) Check all networks defaults to true until we can reliably assign agents to networks to monitor
        if self.init_config.get('check_all_networks', True):
            network_ids = list(set(self.get_all_network_ids()) - set(self.init_config.get('exclude_network_ids', [])))
        else:
            network_ids = self.init_config.get('network_ids', [])

        if not network_ids:
            self.warning("Your check is not configured to monitor any networks.\n" +
                         "Please list `network_ids` under your init_config")

        for nid in network_ids:
            self.get_stats_for_single_network(nid)

    def get_all_network_ids(self):
        url = '{0}/{1}/networks'.format(self.get_neutron_endpoint(), DEFAULT_NEUTRON_API_VERSION)
        headers = {'X-Auth-Token': self.get_auth_token()}

        network_ids = []
        try:
            net_details = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)
            for network in net_details['networks']:
                network_ids.append(network['id'])
        except Exception as e:
            self.warning('Unable to get the list of all network ids: {0}'.format(str(e)))
        return network_ids

    def get_stats_for_single_network(self, network_id):
        url = '{0}/{1}/networks/{2}'.format(self.get_neutron_endpoint(), DEFAULT_NEUTRON_API_VERSION, network_id)
        headers = {'X-Auth-Token': self.get_auth_token()}
        net_details = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)

        service_check_tags = ['network:{0}'.format(network_id)]

        network_name = net_details.get('network', {}).get('name')
        if network_name is not None:
            service_check_tags.append('network_name:{0}'.format(network_name))

        tenant_id = net_details.get('network', {}).get('tenant_id')
        if tenant_id is not None:
            service_check_tags.append('tenant_id:{0}'.format(tenant_id))

        if net_details.get('network', {}).get('admin_state_up'):
            self.service_check(self.NETWORK_SC, AgentCheck.OK, tags=service_check_tags)
        else:
            self.service_check(self.NETWORK_SC, AgentCheck.CRITICAL, tags=service_check_tags)
    ###

    ### Compute
    def get_nova_endpoint(self, instance=None):
        if not instance:
            # Assume instance scope is populated on self
            return self._current_scope.service_catalog.nova_endpoint

        return self.get_scope_for_instance(instance).service_catalog.nova_endpoint

    def _parse_uptime_string(self, uptime):
        """ Parse u' 16:53:48 up 1 day, 21:34,  3 users,  load average: 0.04, 0.14, 0.19\n' """
        uptime = uptime.strip()
        load_averages = uptime[uptime.find('load average:'):].split(':')[1].split(',')
        uptime_sec = uptime.split(',')[0]

        return {
            'loads': map(float, load_averages),
            'uptime_sec': uptime_sec
        }


    def get_all_hypervisor_ids(self, filter_by_host=None):
        nova_version = self.init_config.get("nova_api_version", DEFAULT_NOVA_API_VERSION)
        if nova_version == "v2.1":
            url = '{0}/os-hypervisors'.format(self.get_nova_endpoint())
            headers = {'X-Auth-Token': self.get_auth_token()}

            hypervisor_ids = []
            try:
                hv_list = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)
                for hv in hv_list['hypervisors']:
                    if filter_by_host and hv['hypervisor_hostname'] == filter_by_host:
                        # Assume one-one relationship between hypervisor and host, return the 1st found
                        return [hv['id']]

                    hypervisor_ids.append(hv['id'])
            except Exception as e:
                self.warning('Unable to get the list of all hypervisors: {0}'.format(str(e)))

            return hypervisor_ids
        else:
            if not self.init_config.get("hypervisor_ids"):
                self.warning("Nova API v2 requires admin privileges to index hypervisors. " +
                             "Please specify the hypervisor you wish to monitor under the `hypervisor_ids` section")
                return []
            return self.init_config.get("hypervisor_ids")

    def get_all_aggregate_hypervisors(self):
        url = '{0}/os-aggregates'.format(self.get_nova_endpoint())
        headers = {'X-Auth-Token': self.get_auth_token()}

        hypervisor_aggregate_map = {}
        try:
            aggregate_list = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)
            for v in aggregate_list['aggregates']:
                for host in v['hosts']:
                    hypervisor_aggregate_map[host] = {
                        'aggregate': v['name'],
                        'availability_zone': v['availability_zone']
                    }

        except Exception as e:
            self.warning('Unable to get the list of aggregates: {0}'.format(str(e)))

        return hypervisor_aggregate_map

    def get_uptime_for_single_hypervisor(self, hyp_id):
        url = '{0}/os-hypervisors/{1}/uptime'.format(self.get_nova_endpoint(), hyp_id)
        headers = {'X-Auth-Token': self.get_auth_token()}

        resp = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)
        uptime = resp['hypervisor']['uptime']
        return self._parse_uptime_string(uptime)

    def get_stats_for_single_hypervisor(self, hyp_id, host_tags=None):
        url = '{0}/os-hypervisors/{1}'.format(self.get_nova_endpoint(), hyp_id)
        headers = {'X-Auth-Token': self.get_auth_token()}
        resp = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)
        hyp = resp['hypervisor']
        host_tags = host_tags or []
        tags = [
            'hypervisor:{0}'.format(hyp['hypervisor_hostname']),
            'hypervisor_id:{0}'.format(hyp['id']),
            'virt_type:{0}'.format(hyp['hypervisor_type'])
        ]
        tags.extend(host_tags)

        try:
            uptime = self.get_uptime_for_single_hypervisor(hyp['id'])
        except Exception as e:
            self.warning('Unable to get uptime for hypervisor {0}: {1}'.format(hyp['id'], str(e)))
            uptime = {}

        hyp_state = hyp.get('state', None)
        if hyp_state is None:
            try:
                # Fall back for pre Nova v2.1 to the uptime response
                if uptime.get('uptime_sec', 0) > 0:
                    hyp_state = self.HYPERVISOR_STATE_UP
                else:
                    hyp_state = self.HYPERVISOR_STATE_DOWN
            except Exception:
                # This creates the AgentCheck.UNKNOWN state
                pass

        if hyp_state is None:
            self.service_check(self.HYPERVISOR_SC, AgentCheck.UNKNOWN,
                               tags=tags)
        elif hyp_state != self.HYPERVISOR_STATE_UP:
            self.service_check(self.HYPERVISOR_SC, AgentCheck.CRITICAL,
                               tags=tags)
        else:
            self.service_check(self.HYPERVISOR_SC, AgentCheck.OK,
                               tags=tags)

        for label, val in hyp.iteritems():
            if label in NOVA_HYPERVISOR_METRICS:
                metric_label = "openstack.nova.{0}".format(label)
                self.gauge(metric_label, val, tags=tags)

        load_averages = uptime.get("loads")
        if load_averages is not None:
            assert len(load_averages) == 3
            for i, avg in enumerate([1, 5, 15]):
                self.gauge('openstack.nova.hypervisor_load.{0}'.format(avg), load_averages[i], tags=tags)

    def get_all_server_ids(self, filter_by_host=None):
        query_params = {}
        if filter_by_host:
            query_params["host"] = filter_by_host

        url = '{0}/servers'.format(self.get_nova_endpoint())
        headers = {'X-Auth-Token': self.get_auth_token()}

        server_ids = []
        try:
            resp = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify, params=query_params)

            server_ids = [s['id'] for s in resp['servers']]
        except Exception as e:
            self.warning('Unable to get the list of all servers: {0}'.format(str(e)))

        return server_ids

    def get_stats_for_single_server(self, server_id, tags=None):
        def _is_valid_metric(label):
            return label in NOVA_SERVER_METRICS or any(seg in label for seg in NOVA_SERVER_INTERFACE_SEGMENTS)

        url = '{0}/servers/{1}/diagnostics'.format(self.get_nova_endpoint(), server_id)
        headers = {'X-Auth-Token': self.get_auth_token()}
        server_stats = {}

        try:
            server_stats = self._make_request_with_auth_fallback(url, headers, verify=self._ssl_verify)
        except InstancePowerOffFailure:
            self.warning("Server %s is powered off and cannot be monitored" % server_id)
        except Exception as e:
            self.warning("Unknown error when monitoring %s : %s" % (server_id, e))

        if server_stats:
            tags = tags or []
            for st in server_stats:
                if _is_valid_metric(st):
                    self.gauge("openstack.nova.server.{0}".format(st.replace("-", "_")), server_stats[st], tags=tags, hostname=server_id)


    def get_stats_for_single_project(self, project):
        def _is_valid_metric(label):
            return label in PROJECT_METRICS

        url = '{0}/limits'.format(self.get_nova_endpoint())
        headers = {'X-Auth-Token': self.get_auth_token()}
        server_stats = self._make_request_with_auth_fallback(url, headers, params={"tenant_id": project['id']})

        tags = ['tenant_id:{0}'.format(project['id'])]
        if 'name' in project:
            tags.append('project_name:{0}'.format(project['name']))

        for st in server_stats['limits']['absolute']:
            if _is_valid_metric(st):
                metric_key = PROJECT_METRICS[st]
                self.gauge("openstack.nova.limits.{0}".format(metric_key), server_stats['limits']['absolute'][st], tags=tags)

    ###

    ### Cache util
    def _is_expired(self, entry):
        assert entry in ["aggregates", "physical_hosts", "hypervisors"]
        ttl = self.CACHE_TTL.get(entry)
        last_fetch_time = getattr(self, self.FETCH_TIME_ACCESSORS.get(entry))
        return datetime.now() - last_fetch_time > timedelta(seconds=ttl)

    def _get_and_set_aggregate_list(self):
        if not self._aggregate_list or self._is_expired("aggregates"):
            self._aggregate_list = self.get_all_aggregate_hypervisors()
            self._last_aggregate_fetch_time = datetime.now()

        return self._aggregate_list
    ###

    def _send_api_service_checks(self, instance_scope):
        # Nova
        headers = {"X-Auth-Token": instance_scope.auth_token}

        try:
            requests.get(instance_scope.service_catalog.nova_endpoint, headers=headers, verify=self._ssl_verify, timeout=DEFAULT_API_REQUEST_TIMEOUT)
            self.service_check(self.COMPUTE_API_SC, AgentCheck.OK, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])
        except (requests.exceptions.HTTPError, requests.exceptions.Timeout, requests.exceptions.ConnectionError):
            self.service_check(self.COMPUTE_API_SC, AgentCheck.CRITICAL, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])

        # Neutron
        try:
            requests.get(instance_scope.service_catalog.neutron_endpoint, headers=headers, verify=self._ssl_verify, timeout=DEFAULT_API_REQUEST_TIMEOUT)
            self.service_check(self.NETWORK_API_SC, AgentCheck.OK, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])
        except (requests.exceptions.HTTPError, requests.exceptions.Timeout, requests.exceptions.ConnectionError):
            self.service_check(self.NETWORK_API_SC, AgentCheck.CRITICAL, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])

    def ensure_auth_scope(self, instance):
        """
        Guarantees a valid auth scope for this instance, and returns it

        Communicates with the identity server and initializes a new scope when one is absent, or has been forcibly removed due to token expiry
        """
        instance_scope = None

        try:
            instance_scope = self.get_scope_for_instance(instance)
        except KeyError:

            # We're missing a project scope for this instance
            # Let's populate it now
            try:
                instance_scope = OpenStackProjectScope.from_config(self.init_config, instance)
                self.service_check(self.IDENTITY_API_SC, AgentCheck.OK, tags=["server:%s" % self.init_config.get("keystone_server_url")])
            except KeystoneUnreachable:
                self.warning("The agent could not contact the specified identity server at %s . Are you sure it is up at that address?" % self.init_config.get("keystone_server_url"))
                self.service_check(self.IDENTITY_API_SC, AgentCheck.CRITICAL, tags=["server:%s" % self.init_config.get("keystone_server_url")])

                # If Keystone is down/unreachable, we default the Nova and Neutron APIs to UNKNOWN since we cannot access the service catalog
                self.service_check(self.NETWORK_API_SC, AgentCheck.UNKNOWN, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])
                self.service_check(self.COMPUTE_API_SC, AgentCheck.UNKNOWN, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])

            except MissingNovaEndpoint:
                self.warning("The agent could not find a compatible Nova endpoint in your service catalog!")
                self.service_check(self.COMPUTE_API_SC, AgentCheck.CRITICAL, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])

            except MissingNeutronEndpoint:
                self.warning("The agent could not find a compatible Neutron endpoint in your service catalog!")
                self.service_check(self.NETWORK_API_SC, AgentCheck.CRITICAL, tags=["keystone_server:%s" % self.init_config.get("keystone_server_url")])
            else:
                self.set_scope_for_instance(instance, instance_scope)

        return instance_scope

    def check(self, instance):

        try:
            instance_scope = self.ensure_auth_scope(instance)

            if not instance_scope:
                # Fast fail in the absence of an instance_scope
                return

            self._send_api_service_checks(instance_scope)
            # Store the scope on the object so we don't have to keep passing it around
            self._current_scope = instance_scope

            self.log.debug("Running check with credentials: \n")
            self.log.debug("Nova Url: %s", self.get_nova_endpoint())
            self.log.debug("Neutron Url: %s", self.get_neutron_endpoint())
            self.log.debug("Auth Token: %s", self.get_auth_token())

            # Restrict monitoring to this (host, hypervisor, project)
            # and it's guest servers

            hyp = self.get_local_hypervisor()
            project = self.get_scoped_project(instance)

            # Restrict monitoring to non-excluded servers
            excluded_server_ids = self.init_config.get("exclude_server_ids", [])
            servers = list(
                set(self.get_servers_managed_by_hypervisor()) - set(excluded_server_ids)
            )

            host_tags = self._get_tags_for_host()

            for sid in servers:
                server_tags = ["nova_managed_server"]
                if instance_scope.tenant_id:
                    server_tags.append("tenant_id:%s" % instance_scope.tenant_id)

                self.external_host_tags[sid] = host_tags
                self.get_stats_for_single_server(sid, tags=server_tags)

            if hyp:
                self.get_stats_for_single_hypervisor(hyp, host_tags=host_tags)
            else:
                self.warning("Couldn't get hypervisor to monitor for host: %s" % self.get_my_hostname())

            if project:
                self.get_stats_for_single_project(project)

            # For now, monitor all networks
            self.get_network_stats()

        except IncompleteConfig as e:
            if isinstance(e, IncompleteAuthScope):
                self.warning("""Please specify the auth scope via the `auth_scope` variable in your init_config.\n
                             The auth_scope should look like: \n
                            {'project': {'name': 'my_project', 'domain': {'id': 'my_domain_id'}}}\n
                            OR\n
                            {'project': {'id': 'my_project_id'}}
                             """)
            elif isinstance(e, IncompleteIdentity):
                self.warning("Please specify the user via the `user` variable in your init_config.\n" +
                             "This is the user you would use to authenticate with Keystone v3 via password auth.\n" +
                             "The user should look like: {'password': 'my_password', 'name': 'my_name', 'domain': {'id': 'my_domain_id'}}")
            else:
                self.warning("Configuration Incomplete! Check your openstack.yaml file")


    #### Local Info accessors
    def get_local_hypervisor(self):
        """
        Returns the hypervisor running on this host, and assumes a 1-1 between host and hypervisor
        """
        # Look up hypervisors available filtered by my hostname
        host = self.get_my_hostname()
        hyp = self.get_all_hypervisor_ids(filter_by_host=host)
        if hyp:
            return hyp[0]

    def get_scoped_project(self, instance):
        """
        Returns the project that this instance of the check is scoped to
        """
        project_auth_scope = self.get_scope_for_instance(instance)
        if project_auth_scope.tenant_id:
            return {"id": project_auth_scope.tenant_id}

        filter_params = {
            "name": project_auth_scope.project_name,
            "domain_id": project_auth_scope.domain_id
        }

        url = "{0}/{1}/{2}".format(self.keystone_server_url, DEFAULT_KEYSTONE_API_VERSION, "projects")
        headers = {'X-Auth-Token': self.get_auth_token(instance)}

        try:
            project_details = self._make_request_with_auth_fallback(url, headers, params=filter_params)
            assert len(project_details["projects"]) == 1, "Non-unique project credentials"

            # Set the tenant_id so we won't have to fetch it next time
            project_auth_scope.tenant_id = project_details["projects"][0].get("id")

            return project_details["projects"][0]
        except Exception as e:
            self.warning('Unable to get the list of all project ids: {0}'.format(str(e)))

        return None



    def get_my_hostname(self):
        """
        Returns a best guess for the hostname registered with OpenStack for this host
        """
        return self.init_config.get("os_host") or get_hostname(self.agentConfig)

    def get_servers_managed_by_hypervisor(self):
        return self.get_all_server_ids(filter_by_host=self.get_my_hostname())

    def _get_tags_for_host(self):
        hostname = self.get_my_hostname()

        tags = []
        if hostname in self._get_and_set_aggregate_list():
            tags.append('aggregate:{0}'.format(self._aggregate_list[hostname]['aggregate']))
            # Need to check if there is a value for availability_zone because it is possible to have an aggregate without an AZ
            if self._aggregate_list[hostname]['availability_zone']:
                tags.append('availability_zone:{0}'.format(self._aggregate_list[hostname]['availability_zone']))
        else:
            self.log.info('Unable to find hostname %s in aggregate list. Assuming this host is unaggregated', hostname)

        return tags

    ### For attaching tags to hosts that are not the host running the agent

    def get_external_host_tags(self):
        """ Returns a list of tags for every guest server that is detected by the OpenStack
        integration.
        List of pairs (hostname, list_of_tags)
        """
        self.log.info("Collecting external_host_tags now")
        external_host_tags = []
        for k,v in self.external_host_tags.iteritems():
            external_host_tags.append((k, {SOURCE_TYPE: v}))

        self.log.debug("Sending external_host_tags: %s", external_host_tags)
        return external_host_tags
