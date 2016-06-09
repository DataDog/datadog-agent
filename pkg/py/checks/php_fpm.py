# 3p
import requests

# project
from checks import AgentCheck
from util import headers


class PHPFPMCheck(AgentCheck):
    """
    Tracks basic php-fpm metrics via the status module
    Requires php-fpm pools to have the status option.
    See http://www.php.net/manual/de/install.fpm.configuration.php#pm.status-path for more details
    """

    SERVICE_CHECK_NAME = 'php_fpm.can_ping'

    GAUGES = {
        'listen queue': 'php_fpm.listen_queue.size',
        'idle processes': 'php_fpm.processes.idle',
        'active processes': 'php_fpm.processes.active',
        'total processes': 'php_fpm.processes.total',
    }

    MONOTONIC_COUNTS = {
        'accepted conn': 'php_fpm.requests.accepted',
        'max children reached': 'php_fpm.processes.max_reached',
        'slow requests': 'php_fpm.requests.slow',
    }

    def check(self, instance):
        status_url = instance.get('status_url')
        ping_url = instance.get('ping_url')
        ping_reply = instance.get('ping_reply')

        auth = None
        user = instance.get('user')
        password = instance.get('password')

        tags = instance.get('tags', [])

        if user and password:
            auth = (user, password)

        if status_url is None and ping_url is None:
            raise Exception("No status_url or ping_url specified for this instance")

        pool = None
        status_exception = None
        if status_url is not None:
            try:
                pool = self._process_status(status_url, auth, tags)
            except Exception as e:
                status_exception = e
                pass

        if ping_url is not None:
            self._process_ping(ping_url, ping_reply, auth, tags, pool)

        # pylint doesn't understand that we are raising this only if it's here
        if status_exception is not None:
            raise status_exception  # pylint: disable=E0702

    def _process_status(self, status_url, auth, tags):
        data = {}
        try:
            # TODO: adding the 'full' parameter gets you per-process detailed
            # informations, which could be nice to parse and output as metrics
            resp = requests.get(status_url, auth=auth,
                                headers=headers(self.agentConfig),
                                params={'json': True})
            resp.raise_for_status()

            data = resp.json()
        except Exception as e:
            self.log.error("Failed to get metrics from {0}.\nError {1}".format(status_url, e))
            raise

        pool_name = data.get('pool', 'default')
        metric_tags = tags + ["pool:{0}".format(pool_name)]

        for key, mname in self.GAUGES.iteritems():
            if key not in data:
                self.log.warn("Gauge metric {0} is missing from FPM status".format(key))
                continue
            self.gauge(mname, int(data[key]), tags=metric_tags)

        for key, mname in self.MONOTONIC_COUNTS.iteritems():
            if key not in data:
                self.log.warn("Counter metric {0} is missing from FPM status".format(key))
                continue
            self.monotonic_count(mname, int(data[key]), tags=metric_tags)

        # return pool, to tag the service check with it if we have one
        return pool_name

    def _process_ping(self, ping_url, ping_reply, auth, tags, pool_name):
        if ping_reply is None:
            ping_reply = 'pong'

        sc_tags = ["ping_url:{0}".format(ping_url)]

        try:
            # TODO: adding the 'full' parameter gets you per-process detailed
            # informations, which could be nice to parse and output as metrics
            resp = requests.get(ping_url, auth=auth,
                                headers=headers(self.agentConfig))
            resp.raise_for_status()

            if ping_reply not in resp.text:
                raise Exception("Received unexpected reply to ping {0}".format(resp.text))

        except Exception as e:
            self.log.error("Failed to ping FPM pool {0} on URL {1}."
                           "\nError {2}".format(pool_name, ping_url, e))
            self.service_check(self.SERVICE_CHECK_NAME,
                               AgentCheck.CRITICAL, tags=sc_tags, message=str(e))
        else:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK, tags=sc_tags)
