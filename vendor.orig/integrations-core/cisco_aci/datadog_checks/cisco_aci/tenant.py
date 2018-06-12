# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

import re
import time
import datetime

from . import helpers


class Tenant:
    """
    Collect tenant metrics from the APIC
    """

    def __init__(self, check, api, instance, instance_hash):
        self.check = check
        self.api = api
        self.instance = instance
        self.check_tags = check.check_tags
        self.user_tags = instance.get('tags', [])
        self.instance_hash = instance_hash

        # grab some functions from the check
        self.gauge = check.gauge
        self.rate = check.rate
        self.log = check.log
        self.submit_metrics = check.submit_metrics
        self.tagger = self.check.tagger
        self.tenant_metrics = self.check.tenant_metrics

    def collect(self):
        tenants = self.instance.get('tenant', [])
        if len(tenants) == 0:
            self.log.warning('No tenants were listed in the config, skipping tenant collection')
            return

        self.log.info("collecting from %s tenants" % len(tenants))
        # check if tenant exist before proceeding.
        for t in tenants:
            list_apps = self.api.get_apps(t)
            self.log.info("collecting %s apps from %s" % (len(list_apps), t))
            for app in list_apps:
                self.submit_app_data(t, app)
                app_name = app.get('fvAp', {}).get('attributes', {}).get('name')
                list_epgs = self.api.get_epgs(t, app_name)
                self.log.info("collecting %s endpoint groups from %s" % (len(list_epgs), app_name))
                self.submit_epg_data(t, app_name, list_epgs)
            self.submit_ten_data(t)
            self.collect_events(t)

    def submit_app_data(self, tenant, app):
        a = app.get('fvAp', {})
        attrs = a.get('attributes', {})
        app_name = attrs.get('name')
        if not app_name:
            return
        stats = self.api.get_app_stats(tenant, app_name)
        tags = self.tagger.get_tags(a, 'application')
        self.submit_raw_obj(stats, tags, 'application')

    def submit_epg_data(self, tenant, app, epgs):
        for epg_data in epgs:
            epg = epg_data.get('fvAEPg', {})
            attrs = epg.get('attributes', {})
            epg_name = attrs.get('name')
            if not epg_name:
                continue
            stats = self.api.get_epg_stats(tenant, app, epg_name)
            tags = self.tagger.get_tags(epg, 'endpoint_group')
            self.submit_raw_obj(stats, tags, 'endpoint_group')

    def submit_ten_data(self, tenant):
        stats = self.api.get_tenant_stats(tenant)
        tags = self.tagger.get_tags(tenant, 'tenant')
        self.submit_raw_obj(stats, tags, 'tenant')

    def submit_raw_obj(self, raw_stats, tags, obj_type):
        for s in raw_stats:
            name = s.keys()[0]
            # we only want to collect the 15 minutes metrics.
            if '15min' not in name:
                continue

            attrs = s[name]['attributes']
            if 'index' in attrs:
                continue

            self.log.debug("submitting metrics for: {}".format(name))
            metrics = {}

            tenant_metrics = self.tenant_metrics.get(obj_type, {})

            for n, ms in tenant_metrics.iteritems():
                if n not in name:
                    continue
                for cisco_metric, dd_metric in ms.iteritems():
                    mval = s.get(name, {}).get("attributes", {}).get(cisco_metric)
                    json_attrs = s.get(name, {}).get("attributes", {})
                    if mval and helpers.check_metric_can_be_zero(cisco_metric, mval, json_attrs):
                        metrics[dd_metric] = mval

            self.submit_metrics(metrics, tags, instance=self.instance)

    def collect_events(self, tenant, page=0, page_size=15):
        # If there are too many events, it'll break the agent
        # stop sending after it reaches page 10 (150 events per tenant)
        if page >= 10:
            return

        event_list = self.api.get_tenant_events(tenant, page=page, page_size=15)

        now = int(time.time())
        prior_ts = self.last_events_ts.get(tenant)
        time_window = 600
        if prior_ts:
            time_window = now - prior_ts

        self.last_events_ts[tenant] = now

        log_line = "Fetched: {} events".format(len(event_list))
        if len(event_list) > 0:
            created = event_list[0].get('eventRecord', {}).get('attributes', {}).get('created')
            log_line += ", most recent is from: {}".format(created)
        self.log.info(log_line)

        for event in event_list:
            ev = event.get('eventRecord', {}).get('attributes', {})
            created = ev.get('created')
            create_date = re.search('\d{4}-\d{2}-\d{1,2}T\d{2}:\d{2}:\d{2}', created).group(0)

            self.log.debug("ev time: {}".format(created))
            strptime = datetime.datetime.strptime(create_date, '%Y-%m-%dT%H:%M:%S')
            timestamp = (strptime - datetime.datetime(1970, 1, 1)).total_seconds()
            if now - timestamp > time_window:
                return

            self.log.debug("sending an event!")

            title = "The resource: " + ev['affected'] + " emitted an event"
            dn_tags = helpers.get_event_tags_from_dn(ev['dn'])
            tags = [
                "tenant:" + tenant,
            ]
            tags = tags + self.user_tags + self.check_tags
            if 'code' in ev:
                tags.append("code:" + ev['code'])
            if 'user' in ev:
                tags.append("user:" + ev['user'])
            if 'cause' in ev:
                tags.append("cause:" + ev['cause'])
            if 'severity' in ev:
                tags.append("severity:" + ev['severity'])
            self.event({
                'timestamp': timestamp,
                'event_type': 'cisco_aci',
                'msg_title': title,
                'msg_text': ev['descr'],
                "tags": tags + dn_tags,
                "aggregation_key": ev['id'],
                'host': self.hostname
            })

        # if we get to the end without running out of new events, move onto the next page
        # there is a bug when sometimes it'll return 30 events despite the page size setting
        if len(event_list) != 0 and len(event_list) % 15 == 0:
            self.collect_events(tenant, page=page+1, page_size=15)

    @property
    def last_events_ts(self):
        if self.instance_hash not in self.check.last_events_ts:
            self.check.last_events_ts[self.instance_hash] = {}

        return self.check.last_events_ts[self.instance_hash]
