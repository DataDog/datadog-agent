# stdlib
import time

# 3p
import requests

# project
from checks import AgentCheck
from config import _is_affirmative


class TeamCityCheck(AgentCheck):
    HEADERS = {'Accept': 'application/json'}
    DEFAULT_TIMEOUT = 10
    NEW_BUILD_URL = "http://{server}/guestAuth/app/rest/builds/?locator=buildType:{build_conf},sinceBuild:id:{since_build},status:SUCCESS"
    LAST_BUILD_URL = "http://{server}/guestAuth/app/rest/builds/?locator=buildType:{build_conf},count:1"

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)

        # Keep track of last build IDs per instance
        self.last_build_ids = {}

    def _initialize_if_required(self, instance_name, server, build_conf):
        # Already initialized
        if instance_name in self.last_build_ids:
            return

        self.log.debug("Initializing {0}".format(instance_name))
        build_url = self.LAST_BUILD_URL.format(
            server=server,
            build_conf=build_conf
        )
        try:
            resp = requests.get(build_url, timeout=self.DEFAULT_TIMEOUT, headers=self.HEADERS)
            resp.raise_for_status()

            last_build_id = resp.json().get('build')[0].get('id')
        except requests.exceptions.HTTPError:
            if resp.status_code == 401:
                self.log.error("Access denied. You must enable guest authentication")
            self.log.error(
                "Failed to retrieve last build ID with code {0} for instance '{1}'"
                .format(resp.status_code, instance_name)
            )
            raise
        except Exception:
            self.log.exception(
                "Unhandled exception to get last build ID for instance '{0}'"
                .format(instance_name)
            )
            raise

        self.log.debug(
            "Last build id for instance {0} is {1}."
            .format(instance_name, last_build_id)
        )
        self.last_build_ids[instance_name] = last_build_id

    def _build_and_send_event(self, new_build, instance_name, is_deployment, host, tags):
        self.log.debug("Found new build with id {0}, saving and alerting.".format(new_build["id"]))
        self.last_build_ids[instance_name] = new_build["id"]

        event_dict = {
            'timestamp': int(time.time()),
            'source_type_name': 'teamcity',
            'host': host,
            'tags': [],
        }
        if is_deployment:
            event_dict['event_type'] = 'teamcity_deployment'
            event_dict['msg_title'] = "{0} deployed to {1}".format(instance_name, host)
            event_dict['msg_text'] = "Build Number: {0}\n\nMore Info: {1}".format(new_build["number"],
                                                                                  new_build["webUrl"])
            event_dict['tags'].append('deployment')
        else:
            event_dict['event_type'] = "build"
            event_dict['msg_title'] = "Build for {0} successful".format(instance_name)

            event_dict['msg_text'] = "Build Number: {0}\nDeployed To: {1}\n\nMore Info: {2}".format(new_build["number"],
                                                                                                    host, new_build["webUrl"])
            event_dict['tags'].append('build')

        if tags:
            event_dict["tags"].extend(tags)

        self.event(event_dict)

    def check(self, instance):
        instance_name = instance.get('name')
        if instance_name is None:
            raise Exception("Each instance must have a unique name")

        server = instance.get('server')
        if 'server' is None:
            raise Exception("Each instance must have a server")

        build_conf = instance.get('build_configuration')
        if build_conf is None:
            raise Exception("Each instance must have a build configuration")

        host = instance.get('host_affected') or self.hostname
        tags = instance.get('tags')
        is_deployment = _is_affirmative(instance.get('is_deployment', False))

        self._initialize_if_required(instance_name, server, build_conf)

        # Look for new successful builds
        new_build_url = self.NEW_BUILD_URL.format(
            server=server,
            build_conf=build_conf,
            since_build=self.last_build_ids[instance_name]
        )

        try:
            resp = requests.get(new_build_url, timeout=self.DEFAULT_TIMEOUT, headers=self.HEADERS)
            resp.raise_for_status()

            new_builds = resp.json()

            if new_builds["count"] == 0:
                self.log.debug("No new builds found.")
            else:
                self._build_and_send_event(new_builds["build"][0], instance_name, is_deployment, host, tags)
        except requests.exceptions.HTTPError:
            self.log.exception(
                "Couldn't fetch last build, got code {0}"
                .format(resp.status_code)
            )
            raise
        except Exception:
            self.log.exception(
                "Couldn't fetch last build, unhandled exception"
            )
            raise
