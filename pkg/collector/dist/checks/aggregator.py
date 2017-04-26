# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import logging
from time import time

# project
from checks.metric_types import MetricTypes

log = logging.getLogger(__name__)


# This is used to ensure that metrics with a timestamp older than
# RECENT_POINT_THRESHOLD_DEFAULT seconds (or the value passed in to
# the MetricsAggregator constructor) get discarded rather than being
# input into the incorrect bucket. Currently, the MetricsAggregator
# does not support submitting values for the past, and all values get
# submitted for the timestamp passed into the flush() function.
# The MetricsBucketAggregator uses times that are aligned to "buckets"
# that are the length of the interval that is passed into the
# MetricsBucketAggregator constructor.
RECENT_POINT_THRESHOLD_DEFAULT = 3600


class Infinity(Exception):
    pass


class UnknownValue(Exception):
    pass


class Metric(object):
    """
    A base metric class that accepts points, slices them into time intervals
    and performs roll-ups within those intervals.
    """

    def sample(self, value, sample_rate, timestamp=None):
        """ Add a point to the given metric. """
        raise NotImplementedError()

    def flush(self, timestamp, interval):
        """ Flush all metrics up to the given timestamp. """
        raise NotImplementedError()


class Gauge(Metric):
    """ A metric that tracks a value at particular points in time. """

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.value = None
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.last_sample_time = None
        self.timestamp = time()

    def sample(self, value, sample_rate, timestamp=None):
        self.value = value
        self.last_sample_time = time()
        self.timestamp = timestamp


    def flush(self, timestamp, interval):
        if self.value is not None:
            res = [self.formatter(
                metric=self.name,
                timestamp=self.timestamp or timestamp,
                value=self.value,
                tags=self.tags,
                hostname=self.hostname,
                device_name=self.device_name,
                metric_type=MetricTypes.GAUGE,
                interval=interval,
            )]
            self.value = None
            return res

        return []

class BucketGauge(Gauge):
    """ A metric that tracks a value at particular points in time.
    The difference beween this class and Gauge is that this class will
    report that gauge sample time as the time that Metric is flushed, as
    opposed to the time that the sample was collected.

    """

    def flush(self, timestamp, interval):
        if self.value is not None:
            res = [self.formatter(
                metric=self.name,
                timestamp=timestamp,
                value=self.value,
                tags=self.tags,
                hostname=self.hostname,
                device_name=self.device_name,
                metric_type=MetricTypes.GAUGE,
                interval=interval,
            )]
            self.value = None
            return res

        return []


class Count(Metric):
    """ A metric that tracks a count. """

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.value = None
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.last_sample_time = None

    def sample(self, value, sample_rate, timestamp=None):
        self.value = (self.value or 0) + value
        self.last_sample_time = time()

    def flush(self, timestamp, interval):
        if self.value is None:
            return []
        try:
            return [self.formatter(
                metric=self.name,
                value=self.value,
                timestamp=timestamp,
                tags=self.tags,
                hostname=self.hostname,
                device_name=self.device_name,
                metric_type=MetricTypes.COUNT,
                interval=interval,
            )]
        finally:
            self.value = None

class MonotonicCount(Metric):

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.prev_counter = None
        self.curr_counter = None
        self.count = None
        self.last_sample_time = None

    def sample(self, value, sample_rate, timestamp=None):
        if self.curr_counter is None:
            self.curr_counter = value
        else:
            self.prev_counter = self.curr_counter
            self.curr_counter = value

        prev = self.prev_counter
        curr = self.curr_counter
        if prev is not None and curr is not None:
            self.count = (self.count or 0) + max(0, curr - prev)

        self.last_sample_time = time()

    def flush(self, timestamp, interval):
        if self.count is None:
            return []
        try:
            return [self.formatter(
                hostname=self.hostname,
                device_name=self.device_name,
                tags=self.tags,
                metric=self.name,
                value=self.count,
                timestamp=timestamp,
                metric_type=MetricTypes.COUNT,
                interval=interval
            )]
        finally:
            self.prev_counter = self.curr_counter
            self.curr_counter = None
            self.count = None


class Counter(Metric):
    """ A metric that tracks a counter value. """

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.value = 0
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.last_sample_time = None

    def sample(self, value, sample_rate, timestamp=None):
        self.value += value * int(1 / sample_rate)
        self.last_sample_time = time()

    def flush(self, timestamp, interval):
        try:
            value = self.value / interval
            return [self.formatter(
                metric=self.name,
                value=value,
                timestamp=timestamp,
                tags=self.tags,
                hostname=self.hostname,
                device_name=self.device_name,
                metric_type=MetricTypes.RATE,
                interval=interval,
            )]
        finally:
            self.value = 0

DEFAULT_HISTOGRAM_AGGREGATES = ['max', 'median', 'avg', 'count']
DEFAULT_HISTOGRAM_PERCENTILES = [0.95]

class Histogram(Metric):
    """ A metric to track the distribution of a set of values. """

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.count = 0
        self.samples = []
        self.aggregates = extra_config['aggregates'] if\
            extra_config is not None and extra_config.get('aggregates') is not None\
            else DEFAULT_HISTOGRAM_AGGREGATES
        self.percentiles = extra_config['percentiles'] if\
            extra_config is not None and extra_config.get('percentiles') is not None\
            else DEFAULT_HISTOGRAM_PERCENTILES
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.last_sample_time = None

    def sample(self, value, sample_rate, timestamp=None):
        self.count += int(1 / sample_rate)
        self.samples.append(value)
        self.last_sample_time = time()

    def flush(self, ts, interval):
        if not self.count:
            return []

        self.samples.sort()
        length = len(self.samples)

        min_ = self.samples[0]
        max_ = self.samples[-1]
        med = self.samples[int(round(length/2 - 1))]
        sum_ = sum(self.samples)
        avg = sum_ / float(length)

        aggregators = [
            ('min', min_, MetricTypes.GAUGE),
            ('max', max_, MetricTypes.GAUGE),
            ('median', med, MetricTypes.GAUGE),
            ('avg', avg, MetricTypes.GAUGE),
            ('sum', sum_, MetricTypes.GAUGE),
            ('count', self.count/interval, MetricTypes.RATE),
        ]

        metric_aggrs = [
            (agg_name, agg_func, m_type)
            for agg_name, agg_func, m_type in aggregators
            if agg_name in self.aggregates
        ]

        metrics = [self.formatter(
            hostname=self.hostname,
            device_name=self.device_name,
            tags=self.tags,
            metric='%s.%s' % (self.name, suffix),
            value=value,
            timestamp=ts,
            metric_type=metric_type,
            interval=interval) for suffix, value, metric_type in metric_aggrs
        ]

        for p in self.percentiles:
            val = self.samples[int(round(p * length - 1))]
            name = '%s.%spercentile' % (self.name, int(p * 100))
            metrics.append(self.formatter(
                hostname=self.hostname,
                tags=self.tags,
                metric=name,
                value=val,
                timestamp=ts,
                metric_type=MetricTypes.GAUGE,
                interval=interval,
            ))

        # Reset our state.
        self.samples = []
        self.count = 0

        return metrics


class Set(Metric):
    """ A metric to track the number of unique elements in a set. """

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.values = set()
        self.last_sample_time = None

    def sample(self, value, sample_rate, timestamp=None):
        self.values.add(value)
        self.last_sample_time = time()

    def flush(self, timestamp, interval):
        if not self.values:
            return []
        try:
            return [self.formatter(
                hostname=self.hostname,
                device_name=self.device_name,
                tags=self.tags,
                metric=self.name,
                value=len(self.values),
                timestamp=timestamp,
                metric_type=MetricTypes.GAUGE,
                interval=interval,
            )]
        finally:
            self.values = set()


class Rate(Metric):
    """ Track the rate of metrics over each flush interval """

    def __init__(self, formatter, name, tags, hostname, device_name, extra_config=None):
        self.formatter = formatter
        self.name = name
        self.tags = tags
        self.hostname = hostname
        self.device_name = device_name
        self.samples = []
        self.last_sample_time = None

    def sample(self, value, sample_rate, timestamp=None):
        ts = time()
        self.samples.append((int(ts), value))
        self.last_sample_time = ts

    def _rate(self, sample1, sample2):
        interval = sample2[0] - sample1[0]
        if interval == 0:
            log.warn('Metric %s has an interval of 0. Not flushing.' % self.name)
            raise Infinity()

        delta = sample2[1] - sample1[1]
        if delta < 0:
            log.info('Metric %s has a rate < 0. Counter may have been Reset.' % self.name)
            raise UnknownValue()

        return (delta / float(interval))

    def flush(self, timestamp, interval):
        if len(self.samples) < 2:
            return []
        try:
            try:
                val = self._rate(self.samples[-2], self.samples[-1])
            except Exception:
                return []

            return [self.formatter(
                hostname=self.hostname,
                device_name=self.device_name,
                tags=self.tags,
                metric=self.name,
                value=val,
                timestamp=timestamp,
                metric_type=MetricTypes.GAUGE,
                interval=interval
            )]
        finally:
            self.samples = self.samples[-1:]

class Aggregator(object):
    """
    Abstract metric aggregator class.
    """
    # Types of metrics that allow strings
    ALLOW_STRINGS = ['s', ]

    def __init__(self, hostname, interval=1.0, expiry_seconds=300,
            formatter=None, recent_point_threshold=None,
            histogram_aggregates=None, histogram_percentiles=None,
            utf8_decoding=False):
        self.events = []
        self.service_checks = []
        self.total_count = 0
        self.count = 0
        self.event_count = 0
        self.service_check_count = 0
        self.hostname = hostname
        self.expiry_seconds = expiry_seconds
        self.formatter = formatter or api_formatter
        self.interval = float(interval)

        recent_point_threshold = recent_point_threshold or RECENT_POINT_THRESHOLD_DEFAULT
        self.recent_point_threshold = int(recent_point_threshold)
        self.num_discarded_old_points = 0

        # Additional config passed when instantiating metric configs
        self.metric_config = {
            Histogram: {
                'aggregates': histogram_aggregates,
                'percentiles': histogram_percentiles
            }
        }

        self.utf8_decoding = utf8_decoding

    def packets_per_second(self, interval):
        if interval == 0:
            return 0
        return round(float(self.count)/interval, 2)

    def parse_metric_packet(self, packet):
        """
        Schema of a dogstatsd packet:
        <name>:<value>|<metric_type>|@<sample_rate>|#<tag1_name>:<tag1_value>,<tag2_name>:<tag2_value>:<value>|<metric_type>...
        """
        parsed_packets = []
        name_and_metadata = packet.split(':', 1)

        if len(name_and_metadata) != 2:
            raise Exception(u'Unparseable metric packet: %s' % packet)

        name = name_and_metadata[0]
        broken_split = name_and_metadata[1].split(':')
        data = []
        partial_datum = None
        for token in broken_split:
            # We need to fix the tag groups that got broken by the : split
            if partial_datum is None:
                partial_datum = token
            elif "|" not in token:
                partial_datum += ":" + token
            else:
                data.append(partial_datum)
                partial_datum = token
        data.append(partial_datum)

        for datum in data:
            value_and_metadata = datum.split('|')

            if len(value_and_metadata) < 2:
                raise Exception(u'Unparseable metric packet: %s' % packet)

            # Submit the metric
            raw_value = value_and_metadata[0]
            metric_type = value_and_metadata[1]

            if metric_type in self.ALLOW_STRINGS:
                value = raw_value
            else:
                # Try to cast as an int first to avoid precision issues, then as a
                # float.
                try:
                    value = int(raw_value)
                except ValueError:
                    try:
                        value = float(raw_value)
                    except ValueError:
                        # Otherwise, raise an error saying it must be a number
                        raise Exception(u'Metric value must be a number: %s, %s' % (name, raw_value))

            # Parse the optional values - sample rate & tags.
            sample_rate = 1
            tags = None
            try:
                for m in value_and_metadata[2:]:
                    # Parse the sample rate
                    if m[0] == '@':
                        sample_rate = float(m[1:])
                        # in case it's in a bad state
                        sample_rate = 1 if sample_rate < 0 or sample_rate > 1 else sample_rate
                    elif m[0] == '#':
                        tags = tuple(sorted(m[1:].split(',')))
            except IndexError:
                log.warning(u'Incorrect metric metadata: metric_name:%s, metadata:%s',
                            name, u' '.join(value_and_metadata[2:]))

            parsed_packets.append((name, value, metric_type, tags, sample_rate))

        return parsed_packets

    def _unescape_sc_content(self, string):
        return string.replace('\\n', '\n').replace('m\:', 'm:')

    def _unescape_event_text(self, string):
        return string.replace('\\n', '\n')

    def parse_event_packet(self, packet):
        try:
            name_and_metadata = packet.split(':', 1)
            if len(name_and_metadata) != 2:
                raise Exception(u'Unparseable event packet: %s' % packet)
            # Event syntax:
            # _e{5,4}:title|body|meta
            name = name_and_metadata[0]
            metadata = name_and_metadata[1]
            title_length, text_length = name.split(',')
            title_length = int(title_length[3:])
            text_length = int(text_length[:-1])

            event = {
                'title': metadata[:title_length],
                'text': self._unescape_event_text(metadata[title_length+1:title_length+text_length+1])
            }
            meta = metadata[title_length+text_length+1:]
            for m in meta.split('|')[1:]:
                if m[0] == u't':
                    event['alert_type'] = m[2:]
                elif m[0] == u'k':
                    event['aggregation_key'] = m[2:]
                elif m[0] == u's':
                    event['source_type_name'] = m[2:]
                elif m[0] == u'd':
                    event['date_happened'] = int(m[2:])
                elif m[0] == u'p':
                    event['priority'] = m[2:]
                elif m[0] == u'h':
                    event['hostname'] = m[2:]
                elif m[0] == u'#':
                    event['tags'] = sorted(m[1:].split(u','))
            return event
        except (IndexError, ValueError):
            raise Exception(u'Unparseable event packet: %s' % packet)

    def parse_sc_packet(self, packet):
        try:
            _, data_and_metadata = packet.split('|', 1)
            # Service check syntax:
            # _sc|check_name|status|meta
            if data_and_metadata.count('|') == 1:
                # Case with no metadata
                check_name, status = data_and_metadata.split('|')
                metadata = ''
            else:
                check_name, status, metadata = data_and_metadata.split('|', 2)

            service_check = {
                'check_name': check_name,
                'status': int(status)
            }

            message_delimiter = 'm:' if metadata.startswith('m:') else '|m:'
            if message_delimiter in metadata:
                meta, message = metadata.rsplit(message_delimiter, 1)
                service_check['message'] = self._unescape_sc_content(message)
            else:
                meta = metadata

            if not meta:
                return service_check

            meta = unicode(meta)
            for m in meta.split('|'):
                if m[0] == u'd':
                    service_check['timestamp'] = float(m[2:])
                elif m[0] == u'h':
                    service_check['hostname'] = m[2:]
                elif m[0] == u'#':
                    service_check['tags'] = sorted(m[1:].split(u','))

            return service_check

        except (IndexError, ValueError):
            raise Exception(u'Unparseable service check packet: %s' % packet)

    def submit_packets(self, packets):
        # We should probably consider that packets are always encoded
        # in utf8, but decoding all packets has an perf overhead of 7%
        # So we let the user decide if we wants utf8 by default
        # Keep a very conservative approach anyhow
        # Clients MUST always send UTF-8 encoded content
        if self.utf8_decoding:
            packets = unicode(packets, 'utf-8', errors='replace')

        for packet in packets.splitlines():
            if not packet.strip():
                continue

            if packet.startswith('_e'):
                event = self.parse_event_packet(packet)
                self.event(**event)
                self.event_count += 1
            elif packet.startswith('_sc'):
                service_check = self.parse_sc_packet(packet)
                self.service_check(**service_check)
                self.service_check_count += 1
            else:
                parsed_packets = self.parse_metric_packet(packet)
                self.count += 1
                for name, value, mtype, tags, sample_rate in parsed_packets:
                    hostname, device_name, tags = self._extract_magic_tags(tags)
                    self.submit_metric(name, value, mtype, tags=tags, hostname=hostname,
                                       device_name=device_name, sample_rate=sample_rate)

    def _extract_magic_tags(self, tags):
        """Magic tags (host, device) override metric hostname and device_name attributes"""
        hostname = None
        device_name = None
        # This implementation avoid list operations for the common case
        if tags:
            tags_to_remove = []
            for tag in tags:
                if tag.startswith('host:'):
                    hostname = tag[5:]
                    tags_to_remove.append(tag)
                elif tag.startswith('device:'):
                    device_name = tag[7:]
                    tags_to_remove.append(tag)
            if tags_to_remove:
                # tags is a tuple already sorted, we convert it into a list to pop elements
                tags = list(tags)
                for tag in tags_to_remove:
                    tags.remove(tag)
                tags = tuple(tags) or None
        return hostname, device_name, tags

    def submit_metric(self, name, value, mtype, tags=None, hostname=None,
                      device_name=None, timestamp=None, sample_rate=1):
        """ Add a metric to be aggregated """
        raise NotImplementedError()

    def event(self, title, text, date_happened=None, alert_type=None, aggregation_key=None, source_type_name=None, priority=None, tags=None, hostname=None):
        event = {
            'msg_title': title,
            'msg_text': text,
        }
        if date_happened is not None:
            event['timestamp'] = date_happened
        else:
            event['timestamp'] = int(time())
        if alert_type is not None:
            event['alert_type'] = alert_type
        if aggregation_key is not None:
            event['aggregation_key'] = aggregation_key
        if source_type_name is not None:
            event['source_type_name'] = source_type_name
        if priority is not None:
            event['priority'] = priority
        if tags is not None:
            event['tags'] = sorted(tags)
        if hostname is not None:
            event['host'] = hostname
        else:
            event['host'] = self.hostname

        self.events.append(event)

    def service_check(self, check_name, status, tags=None, timestamp=None,
                      hostname=None, message=None):
        service_check = {
            'check': check_name,
            'status': status,
            'timestamp': timestamp or int(time())
        }
        if tags is not None:
            service_check['tags'] = sorted(tags)

        if hostname is not None:
            service_check['host_name'] = hostname
        else:
            service_check['host_name'] = self.hostname
        if message is not None:
            service_check['message'] = message

        self.service_checks.append(service_check)

    def flush(self):
        """ Flush aggregated metrics """
        raise NotImplementedError()

    def flush_events(self):
        events = self.events
        self.events = []

        self.total_count += self.event_count
        self.event_count = 0

        log.debug("Received %d events since last flush" % len(events))

        return events

    def flush_service_checks(self):
        service_checks = self.service_checks
        self.service_checks = []

        self.total_count += self.service_check_count
        self.service_check_count = 0

        log.debug("Received {0} service check runs since last flush".format(len(service_checks)))

        return service_checks

    def send_packet_count(self, metric_name):
        self.submit_metric(metric_name, self.count, 'g')

class MetricsBucketAggregator(Aggregator):
    """
    A metric aggregator class.
    """

    def __init__(self, hostname, interval=1.0, expiry_seconds=300,
            formatter=None, recent_point_threshold=None,
            histogram_aggregates=None, histogram_percentiles=None,
            utf8_decoding=False):
        super(MetricsBucketAggregator, self).__init__(
            hostname,
            interval,
            expiry_seconds,
            formatter,
            recent_point_threshold,
            histogram_aggregates,
            histogram_percentiles,
            utf8_decoding
        )
        self.metric_by_bucket = {}
        self.last_sample_time_by_context = {}
        self.current_bucket = None
        self.current_mbc = {}
        self.last_flush_cutoff_time = 0
        self.metric_type_to_class = {
            'g': BucketGauge,
            'c': Counter,
            'h': Histogram,
            'ms': Histogram,
            's': Set,
        }

    def calculate_bucket_start(self, timestamp):
        return timestamp - (timestamp % self.interval)

    def submit_metric(self, name, value, mtype, tags=None, hostname=None,
                      device_name=None, timestamp=None, sample_rate=1):
        # Avoid calling extra functions to dedupe tags if there are none
        # Note: if you change the way that context is created, please also change create_empty_metrics,
        #  which counts on this order

        # Keep hostname with empty string to unset it
        hostname = hostname if hostname is not None else self.hostname

        if tags is None:
            context = (name, tuple(), hostname, device_name)
        else:
            context = (name, tuple(sorted(set(tags))), hostname, device_name)

        cur_time = time()
        # Check to make sure that the timestamp that is passed in (if any) is not older than
        #  recent_point_threshold.  If so, discard the point.
        if timestamp is not None and cur_time - int(timestamp) > self.recent_point_threshold:
            log.debug("Discarding %s - ts = %s , current ts = %s " % (name, timestamp, cur_time))
            self.num_discarded_old_points += 1
        else:
            timestamp = timestamp or cur_time
            # Keep track of the buckets using the timestamp at the start time of the bucket
            bucket_start_timestamp = self.calculate_bucket_start(timestamp)
            if bucket_start_timestamp == self.current_bucket:
                metric_by_context = self.current_mbc
            else:
                if bucket_start_timestamp not in self.metric_by_bucket:
                    self.metric_by_bucket[bucket_start_timestamp] = {}
                metric_by_context = self.metric_by_bucket[bucket_start_timestamp]
                self.current_bucket = bucket_start_timestamp
                self.current_mbc = metric_by_context

            if context not in metric_by_context:
                metric_class = self.metric_type_to_class[mtype]
                metric_by_context[context] = metric_class(self.formatter, name, tags,
                    hostname, device_name, self.metric_config.get(metric_class))

            metric_by_context[context].sample(value, sample_rate, timestamp)

    def create_empty_metrics(self, sample_time_by_context, expiry_timestamp, flush_timestamp, metrics):
        # Even if no data is submitted, Counters keep reporting "0" for expiry_seconds.  The other Metrics
        #  (Set, Gauge, Histogram) do not report if no data is submitted
        for context, last_sample_time in sample_time_by_context.items():
            if last_sample_time < expiry_timestamp:
                log.debug("%s hasn't been submitted in %ss. Expiring." % (context, self.expiry_seconds))
                self.last_sample_time_by_context.pop(context, None)
            else:
                # The expiration currently only applies to Counters
                # This counts on the ordering of the context created in submit_metric not changing
                metric = Counter(self.formatter, context[0], context[1], context[2], context[3])
                metrics += metric.flush(flush_timestamp, self.interval)

    def flush(self):
        cur_time = time()
        flush_cutoff_time = self.calculate_bucket_start(cur_time)
        expiry_timestamp = cur_time - self.expiry_seconds

        metrics = []

        if self.metric_by_bucket:
            # We want to process these in order so that we can check for and expired metrics and
            #  re-create non-expired metrics.  We also mutate self.metric_by_bucket.
            for bucket_start_timestamp in sorted(self.metric_by_bucket.keys()):
                metric_by_context = self.metric_by_bucket[bucket_start_timestamp]
                if bucket_start_timestamp < flush_cutoff_time:
                    not_sampled_in_this_bucket = self.last_sample_time_by_context.copy()
                    # We mutate this dictionary while iterating so don't use an iterator.
                    for context, metric in metric_by_context.items():
                        if metric.last_sample_time < expiry_timestamp:
                            # This should never happen
                            log.warning("%s hasn't been submitted in %ss. Expiring." % (context, self.expiry_seconds))
                            not_sampled_in_this_bucket.pop(context, None)
                            self.last_sample_time_by_context.pop(context, None)
                        else:
                            metrics += metric.flush(bucket_start_timestamp, self.interval)
                            if isinstance(metric, Counter):
                                self.last_sample_time_by_context[context] = metric.last_sample_time
                                not_sampled_in_this_bucket.pop(context, None)
                    # We need to account for Metrics that have not expired and were not flushed for this bucket
                    self.create_empty_metrics(not_sampled_in_this_bucket, expiry_timestamp, bucket_start_timestamp, metrics)

                    del self.metric_by_bucket[bucket_start_timestamp]
        else:
            # Even if there are no metrics in this flush, there may be some non-expired counters
            #  We should only create these non-expired metrics if we've passed an interval since the last flush
            if flush_cutoff_time >= self.last_flush_cutoff_time + self.interval:
                self.create_empty_metrics(self.last_sample_time_by_context.copy(), expiry_timestamp,
                                          flush_cutoff_time-self.interval, metrics)

        # Log a warning regarding metrics with old timestamps being submitted
        if self.num_discarded_old_points > 0:
            log.warn('%s points were discarded as a result of having an old timestamp' % self.num_discarded_old_points)
            self.num_discarded_old_points = 0

        # Save some stats.
        log.debug("received %s payloads since last flush" % self.count)
        self.total_count += self.count
        self.count = 0
        self.current_bucket = None
        self.current_mbc = {}
        self.last_flush_cutoff_time = flush_cutoff_time
        return metrics


class MetricsAggregator(Aggregator):
    """
    A metric aggregator class.
    """

    def __init__(self, hostname, interval=1.0, expiry_seconds=300,
            formatter=None, recent_point_threshold=None,
            histogram_aggregates=None, histogram_percentiles=None,
            utf8_decoding=False):
        super(MetricsAggregator, self).__init__(
            hostname,
            interval,
            expiry_seconds,
            formatter,
            recent_point_threshold,
            histogram_aggregates,
            histogram_percentiles,
            utf8_decoding
        )
        self.metrics = {}
        self.metric_type_to_class = {
            'g': Gauge,
            'ct': Count,
            'ct-c': MonotonicCount,
            'c': Counter,
            'h': Histogram,
            'ms': Histogram,
            's': Set,
            '_dd-r': Rate,
        }

    def submit_metric(self, name, value, mtype, tags=None, hostname=None,
                      device_name=None, timestamp=None, sample_rate=1):
        # Avoid calling extra functions to dedupe tags if there are none

        # Keep hostname with empty string to unset it
        hostname = hostname if hostname is not None else self.hostname

        if tags is None:
            context = (name, tuple(), hostname, device_name)
        else:
            context = (name, tuple(sorted(set(tags))), hostname, device_name)
        if context not in self.metrics:
            metric_class = self.metric_type_to_class[mtype]
            self.metrics[context] = metric_class(self.formatter, name, tags,
                hostname, device_name, self.metric_config.get(metric_class))
        cur_time = time()
        if timestamp is not None and cur_time - int(timestamp) > self.recent_point_threshold:
            log.debug("Discarding %s - ts = %s , current ts = %s " % (name, timestamp, cur_time))
            self.num_discarded_old_points += 1
        else:
            self.metrics[context].sample(value, sample_rate, timestamp)

    def gauge(self, name, value, tags=None, hostname=None, device_name=None, timestamp=None):
        self.submit_metric(name, value, 'g', tags, hostname, device_name, timestamp)

    def increment(self, name, value=1, tags=None, hostname=None, device_name=None):
        self.submit_metric(name, value, 'c', tags, hostname, device_name)

    def decrement(self, name, value=-1, tags=None, hostname=None, device_name=None):
        self.submit_metric(name, value, 'c', tags, hostname, device_name)

    def rate(self, name, value, tags=None, hostname=None, device_name=None):
        self.submit_metric(name, value, '_dd-r', tags, hostname, device_name)

    def submit_count(self, name, value, tags=None, hostname=None, device_name=None):
        self.submit_metric(name, value, 'ct', tags, hostname, device_name)

    def count_from_counter(self, name, value, tags=None,
                           hostname=None, device_name=None):
        self.submit_metric(name, value, 'ct-c', tags,
                           hostname, device_name)

    def histogram(self, name, value, tags=None, hostname=None, device_name=None):
        self.submit_metric(name, value, 'h', tags, hostname, device_name)

    def set(self, name, value, tags=None, hostname=None, device_name=None):
        self.submit_metric(name, value, 's', tags, hostname, device_name)

    def flush(self):
        timestamp = time()
        expiry_timestamp = timestamp - self.expiry_seconds

        # Flush points and remove expired metrics. We mutate this dictionary
        # while iterating so don't use an iterator.
        metrics = []
        for context, metric in self.metrics.items():
            if metric.last_sample_time < expiry_timestamp:
                log.debug("%s hasn't been submitted in %ss. Expiring." % (context, self.expiry_seconds))
                del self.metrics[context]
            else:
                metrics += metric.flush(timestamp, self.interval)

        # Log a warning regarding metrics with old timestamps being submitted
        if self.num_discarded_old_points > 0:
            log.warn('%s points were discarded as a result of having an old timestamp' % self.num_discarded_old_points)
            self.num_discarded_old_points = 0

        # Save some stats.
        log.debug("received %s payloads since last flush" % self.count)
        self.total_count += self.count
        self.count = 0
        return metrics

def get_formatter(config):
    formatter = api_formatter

    if config['statsd_metric_namespace']:
        def metric_namespace_formatter_wrapper(metric, value, timestamp, tags,
                                               hostname=None, device_name=None,
                                               metric_type=None, interval=None):

            metric_prefix = config['statsd_metric_namespace']
            if metric_prefix[-1] != '.':
                metric_prefix += '.'

            return api_formatter(metric_prefix + metric, value, timestamp, tags, hostname,
                                 device_name, metric_type, interval)

        formatter = metric_namespace_formatter_wrapper

    return formatter


def api_formatter(metric, value, timestamp, tags, hostname=None, device_name=None,
        metric_type=None, interval=None):
    return {
        'metric': metric,
        'points': [(timestamp, value)],
        'tags': tags,
        'host': hostname,
        'device_name': device_name,
        'type': metric_type or MetricTypes.GAUGE,
        'interval':interval,
    }
