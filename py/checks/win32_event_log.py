'''
Monitor the Windows Event Log
'''
# stdlib
import calendar
from datetime import datetime, timedelta

# 3rd party
import wmi

# project
from checks import AgentCheck

SOURCE_TYPE_NAME = 'event viewer'
EVENT_TYPE = 'win32_log_event'


class Win32EventLog(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig,
                            instances=instances)
        self.last_ts = {}
        self.wmi_conns = {}

    def _get_wmi_conn(self, host, user, password):
        key = "%s:%s:%s" % (host, user, password)
        if key not in self.wmi_conns:
            self.wmi_conns[key] = wmi.WMI(host, user=user, password=password)
        return self.wmi_conns[key]

    def check(self, instance):
        host = instance.get('host')
        user = instance.get('username')
        password = instance.get('password')
        tags = instance.get('tags')
        notify = instance.get('notify', [])
        w = self._get_wmi_conn(host, user, password)

        # Store the last timestamp by instance
        instance_key = self._instance_key(instance)
        if instance_key not in self.last_ts:
            self.last_ts[instance_key] = datetime.utcnow()
            return

        # Find all events in the last check that match our search by running a
        # straight WQL query against the event log
        last_ts = self.last_ts[instance_key]
        q = EventLogQuery(
            ltype=instance.get('type'),
            user=instance.get('user'),
            source_name=instance.get('source_name'),
            log_file=instance.get('log_file'),
            event_id=instance.get('event_id'),
            message_filters=instance.get('message_filters', []),
            start_ts=last_ts
        )
        wql = q.to_wql()
        self.log.debug("Querying for Event Log events: %s" % wql)
        events = w.query(wql)

        # Save any events returned to the payload as Datadog events
        for ev in events:
            log_ev = LogEvent(ev, self.agentConfig.get('api_key', ''),
                              self.hostname, tags, notify,
                              self.init_config.get('tag_event_id', False))

            # Since WQL only compares on the date and NOT the time, we have to
            # do a secondary check to make sure events are after the last
            # timestamp
            if log_ev.is_after(last_ts):
                self.event(log_ev.to_event_dict())
            else:
                self.log.debug('Skipping event after %s. ts=%s' % (last_ts, log_ev.timestamp))

        # Update the last time checked
        self.last_ts[instance_key] = datetime.utcnow()

    def _instance_key(self, instance):
        ''' Generate a unique key per instance for use with keeping track of
            state for each instance.
        '''
        return '%s' % (instance)


class EventLogQuery(object):
    def __init__(self, ltype=None, user=None, source_name=None, log_file=None,
                 event_id=None, start_ts=None, message_filters=None):

        self.filters = [
            ('Type', self._convert_event_types(ltype)),
            ('User', user),
            ('EventCode', event_id),
            ('SourceName', source_name),
            ('LogFile', log_file)
        ]
        self.message_filters = message_filters or []
        self.start_ts = start_ts

    def to_wql(self):
        ''' Return this query as a WQL string. '''
        wql = """
        SELECT Message, SourceName, TimeGenerated, Type, User, InsertionStrings, EventCode
        FROM Win32_NTLogEvent
        WHERE TimeGenerated >= "%s"
        """ % (self._dt_to_wmi(self.start_ts))
        for name, vals in self.filters:
            wql = self._add_filter(name, vals, wql)
        for msg_filter in self.message_filters:
            wql = self._add_message_filter(msg_filter, wql)
        return wql

    def _add_filter(self, name, vals, q):
        if not vals:
            return q
        # A query like (X = Y) does not work, unless there are multiple
        # statements inside the parentheses, such as (X = Y OR Z = Q)
        if len(vals) == 1:
            vals = vals[0]
        if not isinstance(vals, list):
            q += '\nAND %s = "%s"' % (name, vals)
        else:
            q += "\nAND (%s)" % (' OR '.join(
                ['%s = "%s"' % (name, l) for l in vals]
            ))
        return q

    def _add_message_filter(self, msg_filter, q):
        ''' Filter on the message text using a LIKE query. If the filter starts
            with '-' then we'll assume that it's a NOT LIKE filter.
        '''
        if msg_filter.startswith('-'):
            msg_filter = msg_filter[1:]
            q += '\nAND NOT Message LIKE "%s"' % msg_filter
        else:
            q += '\nAND Message LIKE "%s"' % msg_filter
        return q

    def _dt_to_wmi(self, dt):
        ''' A wrapper around wmi.from_time to get a WMI-formatted time from a
            time struct.
        '''
        return wmi.from_time(year=dt.year, month=dt.month, day=dt.day,
                             hours=dt.hour, minutes=dt.minute,
                             seconds=dt.second, microseconds=0, timezone=0)

    def _convert_event_types(self, types):
        ''' Detect if we are running on <= Server 2003. If so, we should convert
            the EventType values to integers
        '''
        return types


class LogEvent(object):
    def __init__(self, ev, api_key, hostname, tags, notify_list, tag_event_id):
        self.event = ev
        self.api_key = api_key
        self.hostname = hostname
        self.tags = self._tags(tags, ev.EventCode) if tag_event_id else tags
        self.notify_list = notify_list
        self.timestamp = self._wmi_to_ts(self.event.TimeGenerated)

    def to_event_dict(self):
        return {
            'timestamp': self.timestamp,
            'event_type': EVENT_TYPE,
            'api_key': self.api_key,
            'msg_title': self._msg_title(self.event),
            'msg_text': self._msg_text(self.event).strip(),
            'aggregation_key': self._aggregation_key(self.event),
            'alert_type': self._alert_type(self.event),
            'source_type_name': SOURCE_TYPE_NAME,
            'host': self.hostname,
            'tags': self.tags
        }

    def is_after(self, ts):
        ''' Compare this event's timestamp to a give timestamp. '''
        if self.timestamp >= int(calendar.timegm(ts.timetuple())):
            return True
        return False

    def _wmi_to_ts(self, wmi_ts):
        ''' Convert a wmi formatted timestamp into an epoch using wmi.to_time().
        '''
        year, month, day, hour, minute, second, microsecond, tz = wmi.to_time(wmi_ts)
        tz_delta = timedelta(minutes=int(tz))
        if '+' in wmi_ts:
            tz_delta = - tz_delta

        dt = datetime(year=year, month=month, day=day, hour=hour, minute=minute,
                      second=second, microsecond=microsecond) + tz_delta
        return int(calendar.timegm(dt.timetuple()))

    def _tags(self, tags, event_code):
        ''' Inject additional tags into the list already supplied to LogEvent.
        '''
        tags_list = []
        if tags is not None:
            tags_list += list(tags)
        tags_list.append("event_id:{event_id}".format(event_id=event_code))
        return tags_list

    def _msg_title(self, event):
        return '%s/%s' % (event.Logfile, event.SourceName)

    def _msg_text(self, event):
        msg_text = ""
        if event.Message:
            msg_text = "%s\n" % event.Message
        elif event.InsertionStrings:
            msg_text = "\n".join([i_str for i_str in event.InsertionStrings
                                  if i_str.strip()])

        if self.notify_list:
            msg_text += "\n%s" % ' '.join([" @" + n for n in self.notify_list])

        return msg_text

    def _alert_type(self, event):
        event_type = event.Type
        # Convert to a Datadog alert type
        if event_type == 'Warning':
            return 'warning'
        elif event_type == 'Error':
            return 'error'
        return 'info'

    def _aggregation_key(self, event):
        return event.SourceName
