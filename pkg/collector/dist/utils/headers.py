def headers(agentConfig, **kwargs):
    # Build the request headers
    res = {
        'User-Agent': 'Datadog Agent/%s' % agentConfig['version'],
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'text/html, */*',
    }
    if 'http_host' in kwargs:
        res['Host'] = kwargs['http_host']
    return res
