# stdlib
import urlparse

# 3rd party
import requests

# project
from checks import AgentCheck
from util import headers


class Fluentd(AgentCheck):
    SERVICE_CHECK_NAME = 'fluentd.is_ok'
    GAUGES = ['retry_count', 'buffer_total_queued_size', 'buffer_queue_length']
    _AVAILABLE_TAGS = frozenset(['plugin_id', 'type'])

    """Tracks basic fluentd metrics via the monitor_agent plugin
    * number of retry_count
    * number of buffer_queue_length
    * number of buffer_total_queued_size

    $ curl http://localhost:24220/api/plugins.json
    {"plugins":[{"type": "monitor_agent", ...}, {"type": "forward", ...}]}
    """
    def check(self, instance):
        if 'monitor_agent_url' not in instance:
            raise Exception('Fluentd instance missing "monitor_agent_url" value.')

        try:
            url = instance.get('monitor_agent_url')
            plugin_ids = instance.get('plugin_ids', [])

            # Fallback  with `tag_by: plugin_id`
            tag_by = instance.get('tag_by')
            tag_by = tag_by if tag_by in self._AVAILABLE_TAGS else 'plugin_id'

            parsed_url = urlparse.urlparse(url)
            monitor_agent_host = parsed_url.hostname
            monitor_agent_port = parsed_url.port or 24220
            service_check_tags = ['fluentd_host:%s' % monitor_agent_host, 'fluentd_port:%s'
                                  % monitor_agent_port]

            r = requests.get(url, headers=headers(self.agentConfig))
            r.raise_for_status()
            status = r.json()

            for p in status['plugins']:
                tag = "%s:%s" % (tag_by, p.get(tag_by))
                for m in self.GAUGES:
                    if p.get(m) is None:
                        continue
                    # Filter unspecified plugins to keep backward compatibility.
                    if len(plugin_ids) == 0 or p.get('plugin_id') in plugin_ids:
                        self.gauge('fluentd.%s' % (m), p.get(m), [tag])
        except Exception, e:
            msg = "No stats could be retrieved from %s : %s" % (url, str(e))
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                               tags=service_check_tags, message=msg)
            raise
        else:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK, tags=service_check_tags)
