# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# project
from checks import AgentCheck

from checks.libs.wmi.sampler import WMISampler

from collections import namedtuple

WMIMetric = namedtuple('WMIMetric', ['name', 'value', 'tags'])


class InvalidWMIQuery(Exception):
    """
    Invalid WMI Query.
    """
    pass


class MissingTagBy(Exception):
    """
    WMI query returned multiple rows but no `tag_by` value was given.
    """
    pass


class TagQueryUniquenessFailure(Exception):
    """
    'Tagging query' did not return or returned multiple results.
    """
    pass


class WinWMICheck(AgentCheck):
    """
    WMI check.

    Windows only.
    """
    def __init__(self, name, init_config, agentConfig, instances):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.wmi_samplers = {}
        self.wmi_props = {}

    def _format_tag_query(self, sampler, wmi_obj, tag_query):
        """
        Format `tag_query` or raise on incorrect parameters.
        """
        try:
            link_source_property = int(wmi_obj[tag_query[0]])
            target_class = tag_query[1]
            link_target_class_property = tag_query[2]
            target_property = tag_query[3]
        except IndexError:
            self.log.error(
                u"Wrong `tag_queries` parameter format. "
                "Please refer to the configuration file for more information.")
            raise
        except TypeError:
            self.log.error(
                u"Incorrect 'link source property' in `tag_queries` parameter:"
                " `{wmi_property}` is not a property of `{wmi_class}`".format(
                    wmi_property=tag_query[0],
                    wmi_class=sampler.class_name,
                )
            )
            raise

        return target_class, target_property, [{link_target_class_property: link_source_property}]

    def _raise_on_invalid_tag_query_result(self, sampler, wmi_obj, tag_query):
        """
        """
        target_property = sampler.property_names[0]
        target_class = sampler.class_name

        if len(sampler) != 1:
            message = "no result was returned"
            if len(sampler):
                message = "multiple results returned (one expected)"

            self.log.warning(
                u"Failed to extract a tag from `tag_queries` parameter: {reason}."
                " wmi_object={wmi_obj} - query={tag_query}".format(
                    reason=message,
                    wmi_obj=wmi_obj, tag_query=tag_query,
                )
            )
            raise TagQueryUniquenessFailure

        if sampler[0][target_property] is None:
            self.log.error(
                u"Incorrect 'target property' in `tag_queries` parameter:"
                " `{wmi_property}` is empty or is not a property"
                "of `{wmi_class}`".format(
                    wmi_property=target_property,
                    wmi_class=target_class,
                )
            )
            raise TypeError

    def _get_tag_query_tag(self, sampler, wmi_obj, tag_query):
        """
        Design a query based on the given WMIObject to extract a tag.

        Returns: tag or TagQueryUniquenessFailure exception.
        """
        self.log.debug(
            u"`tag_queries` parameter found."
            " wmi_object={wmi_obj} - query={tag_query}".format(
                wmi_obj=wmi_obj, tag_query=tag_query,
            )
        )

        # Extract query information
        target_class, target_property, filters = \
            self._format_tag_query(sampler, wmi_obj, tag_query)

        # Create a specific sampler
        tag_query_sampler = WMISampler(
            self.log,
            target_class, [target_property],
            filters=filters,
            **sampler.connection
        )

        tag_query_sampler.sample()

        # Extract tag
        self._raise_on_invalid_tag_query_result(tag_query_sampler, wmi_obj, tag_query)

        link_value = str(tag_query_sampler[0][target_property]).lower()

        tag = "{tag_name}:{tag_value}".format(
            tag_name=target_property.lower(),
            tag_value="_".join(link_value.split())
        )

        self.log.debug(u"Extracted `tag_queries` tag: '{tag}'".format(tag=tag))
        return tag

    def _extract_metrics(self, wmi_sampler, tag_by, tag_queries, constant_tags):
        """
        Extract and tag metrics from the WMISampler.

        Raise when multiple WMIObject were returned by the sampler with no `tag_by` specified.

        Returns: List of WMIMetric
        ```
        [
            WMIMetric("freemegabytes", 19742, ["name:_total"]),
            WMIMetric("avgdiskbytesperwrite", 1536, ["name:c:"]),
        ]
        ```
        """
        if len(wmi_sampler) > 1 and not tag_by:
            raise MissingTagBy(
                u"WMI query returned multiple rows but no `tag_by` value was given."
                " class={wmi_class} - properties={wmi_properties} - filters={filters}".format(
                    wmi_class=wmi_sampler.class_name, wmi_properties=wmi_sampler.property_names,
                    filters=wmi_sampler.filters,
                )
            )

        metrics = []
        tag_by = tag_by.lower()

        for wmi_obj in wmi_sampler:
            tags = list(constant_tags) if constant_tags else []

            # Tag with `tag_queries` parameter
            for query in tag_queries:
                try:
                    tags.append(self._get_tag_query_tag(wmi_sampler, wmi_obj, query))
                except TagQueryUniquenessFailure:
                    continue

            for wmi_property, wmi_value in wmi_obj.iteritems():
                # Tag with `tag_by` parameter
                if wmi_property == tag_by:
                    tag_value = str(wmi_value).lower()
                    if tag_queries and tag_value.find("#") > 0:
                        tag_value = tag_value[:tag_value.find("#")]

                    tags.append(
                        "{name}:{value}".format(
                            name=tag_by, value=tag_value
                        )
                    )
                    continue

                # No metric extraction on 'Name' property
                if wmi_property == 'name':
                    continue

                try:
                    metrics.append(WMIMetric(wmi_property, float(wmi_value), tags))
                except ValueError:
                    self.log.warning(u"When extracting metrics with WMI, found a non digit value"
                                     " for property '{0}'.".format(wmi_property))
                    continue
                except TypeError:
                    self.log.warning(u"When extracting metrics with WMI, found a missing property"
                                     " '{0}'".format(wmi_property))
                    continue
        return metrics

    def _submit_metrics(self, metrics, metric_name_and_type_by_property):
        """
        Resolve metric names and types and submit it.
        """
        for metric in metrics:
            if metric.name not in metric_name_and_type_by_property:
                # Only report the metrics that were specified in the configration
                # Ignore added properties like 'Timestamp_Sys100NS', `Frequency_Sys100NS`, etc ...
                continue

            metric_name, metric_type = metric_name_and_type_by_property[metric.name]
            try:
                func = getattr(self, metric_type.lower())
            except AttributeError:
                raise Exception(u"Invalid metric type: {0}".format(metric_type))

            func(metric_name, metric.value, metric.tags)

    def _get_instance_key(self, host, namespace, wmi_class, other=None):
        """
        Return an index key for a given instance. Useful for caching.
        """
        if other:
            return "{host}:{namespace}:{wmi_class}-{other}".format(
                host=host, namespace=namespace, wmi_class=wmi_class, other=other
            )

        return "{host}:{namespace}:{wmi_class}".format(
            host=host, namespace=namespace, wmi_class=wmi_class,
        )

    def _get_wmi_sampler(self, instance_key, wmi_class, properties, tag_by="", **kwargs):
        """
        Create and cache a WMISampler for the given (class, properties)
        """
        properties = properties + [tag_by] if tag_by else properties

        if instance_key not in self.wmi_samplers:
            wmi_sampler = WMISampler(self.log, wmi_class, properties, **kwargs)
            self.wmi_samplers[instance_key] = wmi_sampler

        return self.wmi_samplers[instance_key]

    def _get_wmi_properties(self, instance_key, metrics, tag_queries):
        """
        Create and cache a (metric name, metric type) by WMI property map and a property list.
        """
        if instance_key not in self.wmi_props:
            metric_name_by_property = dict(
                (wmi_property.lower(), (metric_name, metric_type))
                for wmi_property, metric_name, metric_type in metrics
            )
            properties = map(lambda x: x[0], metrics + tag_queries)
            self.wmi_props[instance_key] = (metric_name_by_property, properties)

        return self.wmi_props[instance_key]


def from_time(year=None, month=None, day=None, hours=None, minutes=None, seconds=None, microseconds=None, timezone=None):
    """Convenience wrapper to take a series of date/time elements and return a WMI time
    of the form `yyyymmddHHMMSS.mmmmmm+UUU`. All elements may be int, string or
    omitted altogether. If omitted, they will be replaced in the output string
    by a series of stars of the appropriate length.
    :param year: The year element of the date/time
    :param month: The month element of the date/time
    :param day: The day element of the date/time
    :param hours: The hours element of the date/time
    :param minutes: The minutes element of the date/time
    :param seconds: The seconds element of the date/time
    :param microseconds: The microseconds element of the date/time
    :param timezone: The timeezone element of the date/time
    :returns: A WMI datetime string of the form: `yyyymmddHHMMSS.mmmmmm+UUU`
    """
    def str_or_stars(i, length):
        if i is None:
            return "*" * length
        else:
            return str(i).rjust(length, "0")

    wmi_time = ""
    wmi_time += str_or_stars(year, 4)
    wmi_time += str_or_stars(month, 2)
    wmi_time += str_or_stars(day, 2)
    wmi_time += str_or_stars(hours, 2)
    wmi_time += str_or_stars(minutes, 2)
    wmi_time += str_or_stars(seconds, 2)
    wmi_time += "."
    wmi_time += str_or_stars(microseconds, 6)
    if timezone is None:
        wmi_time += "+"
    else:
        try:
            int(timezone)
        except ValueError:
            wmi_time += "+"
        else:
            if timezone >= 0:
                wmi_time += "+"
            else:
                wmi_time += "-"
                timezone = abs(timezone)
                wmi_time += str_or_stars(timezone, 3)

    return wmi_time


def to_time(wmi_time):
    """Convenience wrapper to take a WMI datetime string of the form
    yyyymmddHHMMSS.mmmmmm+UUU and return a 9-tuple containing the
    individual elements, or None where string contains placeholder
    stars.

    :param wmi_time: The WMI datetime string in `yyyymmddHHMMSS.mmmmmm+UUU` format

    :returns: A 9-tuple of (year, month, day, hours, minutes, seconds, microseconds, timezone)
    """

    def int_or_none(s, start, end):
        try:
            return int(s[start:end])
        except ValueError:
            return None

    year = int_or_none(wmi_time, 0, 4)
    month = int_or_none(wmi_time, 4, 6)
    day = int_or_none(wmi_time, 6, 8)
    hours = int_or_none(wmi_time, 8, 10)
    minutes = int_or_none(wmi_time, 10, 12)
    seconds = int_or_none(wmi_time, 12, 14)
    microseconds = int_or_none(wmi_time, 15, 21)
    timezone = wmi_time[22:]

    if timezone == "***":
        timezone = None

    return year, month, day, hours, minutes, seconds, microseconds, timezone
