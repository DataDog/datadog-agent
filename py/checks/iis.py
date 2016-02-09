'''
Check the performance counters from IIS
'''
# 3rd party
import wmi

# project
from checks import AgentCheck


class IIS(AgentCheck):
    METRICS = [
        ('iis.uptime', 'gauge', 'ServiceUptime'),

        # Network
        ('iis.net.bytes_sent', 'rate', 'TotalBytesSent'),
        ('iis.net.bytes_rcvd', 'rate', 'TotalBytesReceived'),
        ('iis.net.bytes_total', 'rate', 'TotalBytesTransferred'),
        ('iis.net.num_connections', 'gauge', 'CurrentConnections'),
        ('iis.net.files_sent', 'rate', 'TotalFilesSent'),
        ('iis.net.files_rcvd', 'rate', 'TotalFilesReceived'),
        ('iis.net.connection_attempts', 'rate', 'TotalConnectionAttemptsAllInstances'),

        # HTTP Methods
        ('iis.httpd_request_method.get', 'rate', 'TotalGetRequests'),
        ('iis.httpd_request_method.post', 'rate', 'TotalPostRequests'),
        ('iis.httpd_request_method.head', 'rate', 'TotalHeadRequests'),
        ('iis.httpd_request_method.put', 'rate', 'TotalPutRequests'),
        ('iis.httpd_request_method.delete', 'rate', 'TotalDeleteRequests'),
        ('iis.httpd_request_method.options', 'rate', 'TotalOptionsRequests'),
        ('iis.httpd_request_method.trace', 'rate', 'TotalTraceRequests'),

        # Errors
        ('iis.errors.not_found', 'rate', 'TotalNotFoundErrors'),
        ('iis.errors.locked', 'rate', 'TotalLockedErrors'),

        # Users
        ('iis.users.anon', 'rate', 'TotalAnonymousUsers'),
        ('iis.users.nonanon', 'rate', 'TotalNonAnonymousUsers'),

        # Requests
        ('iis.requests.cgi', 'rate', 'TotalCGIRequests'),
        ('iis.requests.isapi', 'rate', 'TotalISAPIExtensionRequests'),
    ]
    SERVICE_CHECK = "iis.site_up"

    def __init__(self, name, init_config, agentConfig, instances):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.wmi_conns = {}

    def _get_wmi_conn(self, host, user, password):
        key = "%s:%s:%s" % (host, user, password)
        if key not in self.wmi_conns:
            self.wmi_conns[key] = wmi.WMI(host, user=user, password=password)
        return self.wmi_conns[key]

    def check(self, instance):
        # Connect to the WMI provider
        host = instance.get('host', None)
        user = instance.get('username', None)
        password = instance.get('password', None)
        instance_tags = instance.get('tags', [])
        sites = instance.get('sites', ['_Total'])
        w = self._get_wmi_conn(host, user, password)

        try:
            wmi_cls = w.Win32_PerfFormattedData_W3SVC_WebService()
            if not wmi_cls:
                raise Exception('Missing data from Win32_PerfFormattedData_W3SVC_WebService')
        except Exception:
            self.log.exception('Unable to fetch Win32_PerfFormattedData_W3SVC_WebService class')
            return

        expected_sites = set(sites)
        # Iterate over every IIS site
        for iis_site in wmi_cls:
            # Skip any sites we don't specifically want.
            if iis_site.Name not in sites:
                continue

            # Tag with the site name if we're not using the aggregate
            if iis_site.Name != '_Total':
                tags = instance_tags + ['site:%s' % iis_site.Name]
            else:
                tags = instance_tags

            status = AgentCheck.CRITICAL if iis_site.ServiceUptime == 0 else AgentCheck.OK
            self.service_check("iis.site_up", status, tags=['site:%s' % iis_site.Name])
            expected_sites.remove(iis_site.Name)

            for metric, mtype, wmi_val in self.METRICS:
                if not hasattr(iis_site, wmi_val):
                    if wmi_val == 'TotalBytesTransferred' and hasattr(iis_site,
                                                                      'TotalBytesTransfered'):
                        # Windows 2008 sp2 reports it as TotalbytesTransfered
                        # instead of TotalBytesTransferred (single r)
                        wmi_val = 'TotalBytesTransfered'
                    elif wmi_val == 'TotalConnectionAttemptsAllInstances' \
                            and hasattr(iis_site, 'TotalConnectionAttemptsallinstances'):
                        wmi_val = 'TotalConnectionAttemptsallinstances'
                    else:
                        self.warning("Unable to fetch metric %s. Missing %s in "
                                     "Win32_PerfFormattedData_W3SVC_WebService"
                                     % (metric, wmi_val))
                        continue

                # Submit the metric value with the correct type
                value = float(getattr(iis_site, wmi_val))
                metric_func = getattr(self, mtype)
                metric_func(metric, value, tags=tags)

        for remaining_site in expected_sites:
            self.service_check("iis.site_up", AgentCheck.CRITICAL,
                               tags=['site:%s' % remaining_site])
