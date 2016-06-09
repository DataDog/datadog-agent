# stdlib
from hashlib import md5
import time

# 3rd party
import requests

# project
from checks import AgentCheck


class Mesos(AgentCheck):
    SERVICE_CHECK_NAME = "mesos.can_connect"

    def check(self, instance):
        """
        DEPRECATED:
        This generic Mesosphere check is deprecated not actively developed anymore. It will be
        removed in a future version of the Datadog Agent.
        Please head over to the Mesosphere master and slave specific checks.
        """
        self.warning("This check is deprecated in favor of Mesos master and slave specific checks."
                     " It will be removed in a future version of the Datadog Agent.")

        if 'url' not in instance:
            raise Exception('Mesos instance missing "url" value.')

        # Load values from the instance config
        url = instance['url']
        instance_tags = instance.get('tags', [])
        default_timeout = self.init_config.get('default_timeout', 5)
        timeout = float(instance.get('timeout', default_timeout))

        response = self.get_master_roles(url, timeout)
        if response is not None:
            for role in response['roles']:
                tags = ['role:' + role['name']] + instance_tags
                self.gauge('mesos.role.frameworks', len(role['frameworks']), tags=tags)
                self.gauge('mesos.role.weight', role['weight'], tags=tags)
                resources = role['resources']
                for attr in ['cpus','mem']:
                    if attr in resources:
                        self.gauge('mesos.role.' + attr, resources[attr], tags=tags)

        response = self.get_master_stats(url, timeout)
        if response is not None:
            tags = instance_tags
            for key in iter(response):
                self.gauge('mesos.stats.' + key, response[key], tags=tags)

        response = self.get_master_state(url, timeout)
        if response is not None:
            tags = instance_tags
            for attr in ['deactivated_slaves','failed_tasks','finished_tasks','killed_tasks','lost_tasks','staged_tasks','started_tasks']:
                self.gauge('mesos.state.' + attr, response[attr], tags=tags)

            for framework in response['frameworks']:
                tags = ['framework:' + framework['id']] + instance_tags
                resources = framework['resources']
                for attr in ['cpus','mem']:
                    if attr in resources:
                        self.gauge('mesos.state.framework.' + attr, resources[attr], tags=tags)

            for slave in response['slaves']:
                tags = ['mesos','slave:' + slave['id']] + instance_tags
                resources = slave['resources']
                for attr in ['cpus','mem','disk']:
                    if attr in resources:
                        self.gauge('mesos.state.slave.' + attr, resources[attr], tags=tags)

    def get_master_roles(self, url, timeout):
        return self.get_json(url + "/master/roles.json", timeout)

    def get_master_stats(self, url, timeout):
        return self.get_json(url + "/master/stats.json", timeout)

    def get_master_state(self, url, timeout):
        return self.get_json(url + "/master/state.json", timeout)

    def get_json(self, url, timeout):
        # Use a hash of the URL as an aggregation key
        aggregation_key = md5(url).hexdigest()
        tags = ["url:%s" % url]
        msg = None
        status = None
        try:
            r = requests.get(url, timeout=timeout)
            if r.status_code != 200:
                self.status_code_event(url, r, aggregation_key)
                status = AgentCheck.CRITICAL
                msg = "Got %s when hitting %s" % (r.status_code, url)
            else:
                status = AgentCheck.OK
                msg = "Mesos master instance detected at %s " % url
        except requests.exceptions.Timeout as e:
            # If there's a timeout
            self.timeout_event(url, timeout, aggregation_key)
            msg = "%s seconds timeout when hitting %s" % (timeout, url)
            status = AgentCheck.CRITICAL
        except Exception as e:
            msg = str(e)
            status = AgentCheck.CRITICAL
        finally:
            self.service_check(self.SERVICE_CHECK_NAME, status, tags=tags, message=msg)
            if status is AgentCheck.CRITICAL:
                self.warning(msg)
                return None

        return r.json()

    def timeout_event(self, url, timeout, aggregation_key):
        self.event({
            'timestamp': int(time.time()),
            'event_type': 'http_check',
            'msg_title': 'URL timeout',
            'msg_text': '%s timed out after %s seconds.' % (url, timeout),
            'aggregation_key': aggregation_key
        })

    def status_code_event(self, url, r, aggregation_key):
        self.event({
            'timestamp': int(time.time()),
            'event_type': 'http_check',
            'msg_title': 'Invalid reponse code for %s' % url,
            'msg_text': '%s returned a status of %s' % (url, r.status_code),
            'aggregation_key': aggregation_key
        })
