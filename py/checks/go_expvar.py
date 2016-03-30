# stdlib
from collections import defaultdict
import re

# 3rd party
import requests

# project
from checks import AgentCheck

DEFAULT_MAX_METRICS = 350
PATH = "path"
ALIAS = "alias"
TYPE = "type"
TAGS = "tags"

GAUGE = "gauge"
RATE = "rate"
DEFAULT_TYPE = GAUGE


SUPPORTED_TYPES = {
    GAUGE: AgentCheck.gauge,
    RATE: AgentCheck.rate,
}

DEFAULT_METRIC_NAMESPACE = "go_expvar"


# See http://golang.org/pkg/runtime/#MemStats
DEFAULT_GAUGE_MEMSTAT_METRICS = [
    # General statistics
    "Alloc", "TotalAlloc",

    # Main allocation heap statistics
    "HeapAlloc", "HeapSys", "HeapIdle", "HeapInuse",
    "HeapReleased", "HeapObjects",

]

DEFAULT_RATE_MEMSTAT_METRICS = [
    # General statistics
    "Lookups", "Mallocs", "Frees",

    # Garbage collector statistics
    "PauseTotalNs", "NumGC",
]

DEFAULT_METRICS = [{PATH: "memstats/%s" % path, TYPE: GAUGE} for path in DEFAULT_GAUGE_MEMSTAT_METRICS] +\
    [{PATH: "memstats/%s" % path, TYPE: RATE} for path in DEFAULT_RATE_MEMSTAT_METRICS]


class GoExpvar(AgentCheck):

    def __init__(self, *args, **kwargs):
        super(GoExpvar, self).__init__(*args, **kwargs)
        self._last_gc_count = defaultdict(int)

    def _get_data(self, url):
        r = requests.get(url)
        r.raise_for_status()
        return r.json()

    def _load(self, instance):
        url = instance.get('expvar_url')
        if not url:
            raise Exception('GoExpvar instance missing "expvar_url" value.')

        tags = instance.get('tags', [])
        tags.append("expvar_url:%s" % url)
        data = self._get_data(url)
        metrics = DEFAULT_METRICS + instance.get("metrics", [])
        max_metrics = instance.get("max_returned_metrics", DEFAULT_MAX_METRICS)
        namespace = instance.get('namespace', DEFAULT_METRIC_NAMESPACE)
        return data, tags, metrics, max_metrics, url, namespace

    def get_gc_collection_histogram(self, data, tags, url, namespace):
        num_gc = data.get("memstats", {}).get("NumGC")
        pause_hist = data.get("memstats", {}).get("PauseNs")
        last_gc_count = self._last_gc_count[url]
        if last_gc_count == num_gc:
            # No GC has run. Do nothing
            return
        start = last_gc_count % 256
        end = (num_gc + 255) % 256 + 1
        if start < end:
            values = pause_hist[start:end]
        else:
            values = pause_hist[start:] + pause_hist[:end]

        self._last_gc_count[url] = num_gc

        for value in values:
            self.histogram(
                self.normalize("memstats.PauseNs", namespace, fix_case=True),
                value, tags=tags)

    def check(self, instance):
        data, tags, metrics, max_metrics, url, namespace = self._load(instance)
        self.get_gc_collection_histogram(data, tags, url, namespace)
        self.parse_expvar_data(data, tags, metrics, max_metrics, namespace)

    def parse_expvar_data(self, data, tags, metrics, max_metrics, namespace):
        '''
        Report all the metrics based on the configuration in instance
        If a metric is not well configured or is not present in the payload,
        continue processing metrics but log the information to the info page
        '''
        count = 0
        for metric in metrics:
            path = metric.get(PATH)
            metric_type = metric.get(TYPE, DEFAULT_TYPE)
            metric_tags = list(metric.get(TAGS, []))
            metric_tags += tags
            alias = metric.get(ALIAS)

            if not path:
                self.warning("Metric %s has no path" % metric)
                continue

            if metric_type not in SUPPORTED_TYPES:
                self.warning("Metric type %s not supported for this check" % metric_type)
                continue

            keys = path.split("/")
            values = self.deep_get(data, keys)

            if len(values) == 0:
                self.warning("No results matching path %s" % path)
                continue

            tag_by_path = alias is not None

            for traversed_path, value in values:
                actual_path = ".".join(traversed_path)
                if tag_by_path:
                    metric_tags.append("path:%s" % actual_path)

                metric_name = alias or self.normalize(actual_path, namespace, fix_case=True)

                try:
                    float(value)
                except ValueError:
                    self.log.warning("Unreportable value for path %s: %s" % (path, value))
                    continue

                if count >= max_metrics:
                    self.warning("Reporting more metrics than the allowed maximum. "
                                 "Please contact support@datadoghq.com for more information.")
                    return

                SUPPORTED_TYPES[metric_type](self, metric_name, value, metric_tags)
                count += 1

    def deep_get(self, content, keys, traversed_path=None):
        '''
        Allow to retrieve content nested inside a several layers deep dict/list

        Examples: -content: {
                            "key1": {
                                "key2" : [
                                            {
                                                "name"  : "object1",
                                                "value" : 42
                                            },
                                            {
                                                "name"  : "object2",
                                                "value" : 72
                                            }
                                          ]
                            }
                        }
                  -keys: ["key1", "key2", "1", "value"] would return [(["key1", "key2", "1", "value"], 72)]
                  -keys: ["key1", "key2", "1", "*"] would return [(["key1", "key2", "1", "value"], 72), (["key1", "key2", "1", "name"], "object2")]
                  -keys: ["key1", "key2", "*", "value"] would return [(["key1", "key2", "1", "value"], 72), (["key1", "key2", "0", "value"], 42)]
        '''

        if traversed_path is None:
            traversed_path = []

        if keys == []:
            return [(traversed_path, content)]

        key = keys[0]
        regex = "".join(["^", key, "$"])
        try:
            key_rex = re.compile(regex)
        except Exception:
            self.warning("Cannot compile regex: %s" % regex)
            return []

        results = []
        for new_key, new_content in self.items(content):
            if key_rex.match(new_key):
                results.extend(self.deep_get(new_content, keys[1:], traversed_path + [str(new_key)]))
        return results

    def items(self, object):
        if isinstance(object, list):
            for new_key, new_content in enumerate(object):
                yield str(new_key), new_content
        elif isinstance(object, dict):
            for new_key, new_content in object.iteritems():
                yield str(new_key), new_content
        else:
            self.log.warning("Could not parse this object, check the json"
                             "served by the expvar")
