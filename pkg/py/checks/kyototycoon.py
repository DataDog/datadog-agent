# stdlib
from collections import defaultdict
import re

# 3rd party
import requests

# project
from checks import AgentCheck

db_stats = re.compile(r'^db_(\d)+$')
whitespace = re.compile(r'\s')


class KyotoTycoonCheck(AgentCheck):
    """Report statistics about the Kyoto Tycoon DBM-style
    database server (http://fallabs.com/kyototycoon/)
    """
    SOURCE_TYPE_NAME = 'kyoto tycoon'
    SERVICE_CHECK_NAME = 'kyototycoon.can_connect'

    GAUGES = {
        'repl_delay':         'replication.delay',
        'serv_thread_count':  'threads',
    }

    RATES = {
        'serv_conn_count':    'connections',
        'cnt_get':            'ops.get.hits',
        'cnt_get_misses':     'ops.get.misses',
        'cnt_set':            'ops.set.hits',
        'cnt_set_misses':     'ops.set.misses',
        'cnt_remove':         'ops.del.hits',
        'cnt_remove_misses':  'ops.del.misses',
    }

    DB_GAUGES = {
        'count':              'records',
        'size':               'size',
    }
    TOTALS = {
        'cnt_get':            'ops.get.total',
        'cnt_get_misses':     'ops.get.total',
        'cnt_set':            'ops.set.total',
        'cnt_set_misses':     'ops.set.total',
        'cnt_remove':         'ops.del.total',
        'cnt_remove_misses':  'ops.del.total',
    }

    def check(self, instance):
        url = instance.get('report_url')
        if not url:
            raise Exception('Invalid Kyoto Tycoon report url %r' % url)

        tags = instance.get('tags', {})
        name = instance.get('name')

        # generate the formatted list of tags
        tags = ['%s:%s' % (k, v) for k, v in tags.items()]
        if name is not None:
            tags.append('instance:%s' % name)

        service_check_tags = []
        if name is not None:
            service_check_tags.append('instance:%s' % name)


        try:
            r = requests.get(url)
            r.raise_for_status()
        except requests.exceptions.HTTPError as e:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                tags=service_check_tags, message=str(e.message))
            raise
        except Exception as e:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                tags=service_check_tags, message=str(e))
            raise
        else:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                tags=service_check_tags)

        body = r.content

        totals = defaultdict(int)
        for line in body.splitlines():
            if '\t' not in line:
                continue

            key, value = line.strip().split('\t', 1)
            if key in self.GAUGES:
                name = self.GAUGES[key]
                self.gauge('kyototycoon.%s' % name, float(value), tags=tags)

            elif key in self.RATES:
                name = self.RATES[key]
                self.rate('kyototycoon.%s_per_s' % name, float(value), tags=tags)

            elif db_stats.match(key):
                # Also produce a per-db metrics tagged with the db
                # number in addition to the default tags
                m = db_stats.match(key)
                dbnum = int(m.group(1))
                mytags = tags + ['db:%d' % dbnum]
                for part in whitespace.split(value):
                    k, v = part.split('=', 1)
                    if k in self.DB_GAUGES:
                        name = self.DB_GAUGES[k]
                        self.gauge('kyototycoon.%s' % name, float(v), tags=mytags)

            if key in self.TOTALS:
                totals[self.TOTALS[key]] += float(value)

        for key, value in totals.items():
            self.rate('kyototycoon.%s_per_s' % key, value, tags=tags)
