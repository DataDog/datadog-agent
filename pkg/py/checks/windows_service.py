""" Collect status information for Windows services
"""
# 3rd party
import wmi

# project
from checks import AgentCheck


class WindowsService(AgentCheck):
    STATE_TO_VALUE = {
        'Stopped': AgentCheck.CRITICAL,
        'Start Pending': AgentCheck.WARNING,
        'Stop Pending': AgentCheck.WARNING,
        'Running': AgentCheck.OK,
        'Continue Pending': AgentCheck.WARNING,
        'Pause Pending': AgentCheck.WARNING,
        'Paused': AgentCheck.WARNING,
        'Unknown': AgentCheck.UNKNOWN
    }

    def __init__(self, name, init_config, agentConfig, instances):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.wmi_conns = {}

    def _get_wmi_conn(self, host, user, password):
        key = "%s:%s" % (host, user)
        if key not in self.wmi_conns:
            self.wmi_conns[key] = wmi.WMI(host, user=user, password=password)
        return self.wmi_conns[key]

    def check(self, instance):
        # Connect to the WMI provider
        host = instance.get('host', None)
        user = instance.get('username', None)
        password = instance.get('password', None)
        services = instance.get('services') or []
        w = self._get_wmi_conn(host, user, password)

        if len(services) == 0:
            raise Exception('No services defined in windows_service.yaml')

        for service in services:
            self.log.debug(u"Looking for service name: %s" % service)
            results = w.Win32_Service(name=service)
            if len(results) == 0:
                self.warning(u"No services found matching %s" % service)
                continue
            elif len(results) > 1:
                self.warning(u"Multiple services found matching %s" % service)
                continue

            wmi_service = results[0]
            self._create_service_check(wmi_service, host)

    def _create_service_check(self, wmi_service, host):
        """ Given an instance of a wmi_object from Win32_Service, write any
            performance counters to be gathered and flushed by the collector.
        """
        if host == ".":
            host_name = self.hostname
        else:
            host_name = host

        tags = [u'service:%s' % wmi_service.Name, u'host:%s' % host_name]
        state_value = self.STATE_TO_VALUE.get(wmi_service.State, AgentCheck.UNKNOWN)
        self.service_check('windows_service.state', state_value, tags=tags)
