# stdlib
from collections import defaultdict

# 3rd party
from boto.s3.connection import S3Connection
import simplejson as json

# project
from checks import AgentCheck
from config import _is_affirmative


def multidict(ordered_pairs):
    """Convert duplicate keys values to lists."""
    # read all values into lists
    d = defaultdict(list)
    for k, v in ordered_pairs:
        d[k].append(v)
    # unpack lists that have only 1 item
    for k, v in d.items():
        if len(v) == 1:
            d[k] = v[0]
    return dict(d)


class RiakCs(AgentCheck):

    STATS_BUCKET = 'riak-cs'
    STATS_KEY = 'stats'
    SERVICE_CHECK_NAME = 'riakcs.can_connect'

    def check(self, instance):
        s3, aggregation_key, tags = self._connect(instance)

        stats = self._get_stats(s3, aggregation_key)

        self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
          tags=["aggregation_key:{0}".format(aggregation_key)])

        self.process_stats(stats, tags)

    def process_stats(self, stats, tags):
        if not stats:
            raise Exception("No stats were collected")

        legends = dict([(len(k), k) for k in stats["legend"]])
        del stats["legend"]
        for key, values in stats.iteritems():
            legend = legends[len(values)]
            for i, value in enumerate(values):
                metric_name = "riakcs.{0}.{1}".format(key, legend[i])
                self.gauge(metric_name, value, tags=tags)


    def _connect(self, instance):
        for e in ("access_id", "access_secret"):
            if e not in instance:
                raise Exception("{0} parameter is required.".format(e))

        s3_settings = {
            "aws_access_key_id": instance.get('access_id', None),
            "aws_secret_access_key": instance.get('access_secret', None),
            "proxy": instance.get('host', 'localhost'),
            "proxy_port": int(instance.get('port', 8080)),
            "is_secure": _is_affirmative(instance.get('is_secure', True))
        }

        if instance.get('s3_root'):
            s3_settings['host'] = instance['s3_root']

        aggregation_key = s3_settings['proxy'] + ":" + str(s3_settings['proxy_port'])

        try:
            s3 = S3Connection(**s3_settings)
        except Exception, e:
            self.log.error("Error connecting to {0}: {1}".format(aggregation_key, e))
            self.service_check(
                self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                tags=["aggregation_key:{0}".format(aggregation_key)],
                message=str(e))
            raise

        tags = instance.get("tags", [])
        tags.append("aggregation_key:{0}".format(aggregation_key))

        return s3, aggregation_key, tags

    def _get_stats(self, s3, aggregation_key):
        try:
            bucket = s3.get_bucket(self.STATS_BUCKET, validate=False)
            key = bucket.get_key(self.STATS_KEY)
            stats_str = key.get_contents_as_string()
            stats = self.load_json(stats_str)

        except Exception, e:
            self.log.error("Error retrieving stats from {0}: {1}".format(aggregation_key, e))
            self.service_check(
                self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                tags=["aggregation_key:{0}".format(aggregation_key)],
                message=str(e))
            raise

        return stats


    # We need this as the riak cs stats page returns json with duplicate keys
    @classmethod
    def load_json(cls, text):
        return json.JSONDecoder(object_pairs_hook=multidict).decode(text)
