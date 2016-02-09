# 3rd party
import requests

# project
from checks import AgentCheck
from config import _is_affirmative
from util import headers


class Etcd(AgentCheck):

    DEFAULT_TIMEOUT = 5

    SERVICE_CHECK_NAME = 'etcd.can_connect'

    STORE_RATES = {
        'getsSuccess': 'etcd.store.gets.success',
        'getsFail': 'etcd.store.gets.fail',
        'setsSuccess': 'etcd.store.sets.success',
        'setsFail': 'etcd.store.sets.fail',
        'deleteSuccess': 'etcd.store.delete.success',
        'deleteFail': 'etcd.store.delete.fail',
        'updateSuccess': 'etcd.store.update.success',
        'updateFail': 'etcd.store.update.fail',
        'createSuccess': 'etcd.store.create.success',
        'createFail': 'etcd.store.create.fail',
        'compareAndSwapSuccess': 'etcd.store.compareandswap.success',
        'compareAndSwapFail': 'etcd.store.compareandswap.fail',
        'compareAndDeleteSuccess': 'etcd.store.compareanddelete.success',
        'compareAndDeleteFail': 'etcd.store.compareanddelete.fail',
        'expireCount': 'etcd.store.expire.count'
    }

    STORE_GAUGES = {
        'watchers': 'etcd.store.watchers'
    }

    SELF_GAUGES = {
        'sendPkgRate': 'etcd.self.send.pkgrate',
        'sendBandwidthRate': 'etcd.self.send.bandwidthrate',
        'recvPkgRate': 'etcd.self.recv.pkgrate',
        'recvBandwidthRate': 'etcd.self.recv.bandwidthrate'
    }

    SELF_RATES = {
        'recvAppendRequestCnt': 'etcd.self.recv.appendrequest.count',
        'sendAppendRequestCnt': 'etcd.self.send.appendrequest.count'
    }

    LEADER_COUNTS = {
        # Rates
        'fail': 'etcd.leader.counts.fail',
        'success': 'etcd.leader.counts.success',
    }

    LEADER_LATENCY = {
        # Gauges
        'current': 'etcd.leader.latency.current',
        'average': 'etcd.leader.latency.avg',
        'minimum': 'etcd.leader.latency.min',
        'maximum': 'etcd.leader.latency.max',
        'standardDeviation': 'etcd.leader.latency.stddev',
    }

    def check(self, instance):
        if 'url' not in instance:
            raise Exception('etcd instance missing "url" value.')

        # Load values from the instance config
        url = instance['url']
        instance_tags = instance.get('tags', [])

        # Load the ssl configuration
        ssl_params = {
            'ssl_keyfile': instance.get('ssl_keyfile'),
            'ssl_certfile': instance.get('ssl_certfile'),
            'ssl_cert_validation': _is_affirmative(instance.get('ssl_cert_validation', True)),
            'ssl_ca_certs': instance.get('ssl_ca_certs'),
        }

        for key, param in ssl_params.items():
            if param is None:
                del ssl_params[key]
        # Append the instance's URL in case there are more than one, that
        # way they can tell the difference!
        instance_tags.append("url:{0}".format(url))
        timeout = float(instance.get('timeout', self.DEFAULT_TIMEOUT))
        is_leader = False

        # Gather self metrics
        self_response = self._get_self_metrics(url, ssl_params, timeout)
        if self_response is not None:
            if self_response['state'] == 'StateLeader':
                is_leader = True
                instance_tags.append('etcd_state:leader')
            else:
                instance_tags.append('etcd_state:follower')

            for key in self.SELF_RATES:
                if key in self_response:
                    self.rate(self.SELF_RATES[key], self_response[key], tags=instance_tags)
                else:
                    self.log.warn("Missing key {0} in stats.".format(key))

            for key in self.SELF_GAUGES:
                if key in self_response:
                    self.gauge(self.SELF_GAUGES[key], self_response[key], tags=instance_tags)
                else:
                    self.log.warn("Missing key {0} in stats.".format(key))

        # Gather store metrics
        store_response = self._get_store_metrics(url, ssl_params, timeout)
        if store_response is not None:
            for key in self.STORE_RATES:
                if key in store_response:
                    self.rate(self.STORE_RATES[key], store_response[key], tags=instance_tags)
                else:
                    self.log.warn("Missing key {0} in stats.".format(key))

            for key in self.STORE_GAUGES:
                if key in store_response:
                    self.gauge(self.STORE_GAUGES[key], store_response[key], tags=instance_tags)
                else:
                    self.log.warn("Missing key {0} in stats.".format(key))

        # Gather leader metrics
        if is_leader:
            leader_response = self._get_leader_metrics(url, ssl_params, timeout)
            if leader_response is not None and len(leader_response.get("followers", {})) > 0:
                # Get the followers
                followers = leader_response.get("followers")
                for fol in followers:
                    # counts
                    for key in self.LEADER_COUNTS:
                        self.rate(self.LEADER_COUNTS[key],
                                  followers[fol].get("counts").get(key),
                                  tags=instance_tags + ['follower:{0}'.format(fol)])
                    # latency
                    for key in self.LEADER_LATENCY:
                        self.gauge(self.LEADER_LATENCY[key],
                                   followers[fol].get("latency").get(key),
                                   tags=instance_tags + ['follower:{0}'.format(fol)])

        # Service check
        if self_response is not None and store_response is not None:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                               tags=["url:{0}".format(url)])

    def _get_self_metrics(self, url, ssl_params, timeout):
        return self._get_json(url + "/v2/stats/self",  ssl_params, timeout)

    def _get_store_metrics(self, url, ssl_params, timeout):
        return self._get_json(url + "/v2/stats/store",  ssl_params, timeout)

    def _get_leader_metrics(self, url, ssl_params, timeout):
        return self._get_json(url + "/v2/stats/leader", ssl_params, timeout)

    def _get_json(self, url, ssl_params, timeout):
        try:
            certificate = None
            if 'ssl_certfile' in ssl_params and 'ssl_keyfile' in ssl_params:
                certificate = (ssl_params['ssl_certfile'], ssl_params['ssl_keyfile'])
            verify = ssl_params.get('ssl_ca_certs', True) if ssl_params['ssl_cert_validation'] else False
            r = requests.get(url, verify=verify, cert=certificate, timeout=timeout, headers=headers(self.agentConfig))
        except requests.exceptions.Timeout:
            # If there's a timeout
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                               message="Timeout when hitting %s" % url,
                               tags=["url:{0}".format(url)])
            raise

        if r.status_code != 200:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                               message="Got %s when hitting %s" % (r.status_code, url),
                               tags=["url:{0}".format(url)])
            raise Exception("Http status code {0} on url {1}".format(r.status_code, url))

        return r.json()
