# stdlib
from collections import namedtuple

# project
from checks import AgentCheck
from checks.libs.wmi.sampler import WMISampler

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


class WMICheck(AgentCheck):
    """
    WMI check.

    Windows only.
    """
    def __init__(self, name, init_config, agentConfig, instances):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.wmi_samplers = {}
        self.wmi_props = {}

    def check(self, instance):
        """
        Fetch WMI metrics.
        """
        # Connection information
        host = instance.get('host', "localhost")
        namespace = instance.get('namespace', "root\\cimv2")
        username = instance.get('username', "")
        password = instance.get('password', "")

        # WMI instance
        wmi_class = instance.get('class')
        metrics = instance.get('metrics')
        filters = instance.get('filters')
        tag_by = instance.get('tag_by', "").lower()
        tag_queries = instance.get('tag_queries', [])
        constant_tags = instance.get('constant_tags')

        # Create or retrieve an existing WMISampler
        instance_key = self._get_instance_key(host, namespace, wmi_class)

        metric_name_and_type_by_property, properties = \
            self._get_wmi_properties(instance_key, metrics, tag_queries)

        wmi_sampler = self._get_wmi_sampler(
            instance_key,
            wmi_class, properties,
            filters=filters,
            host=host, namespace=namespace,
            username=username, password=password
        )

        # Sample, extract & submit metrics
        wmi_sampler.sample()
        metrics = self._extract_metrics(wmi_sampler, tag_by, tag_queries, constant_tags)
        self._submit_metrics(metrics, metric_name_and_type_by_property)

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
                " `{wmi_property}` is not a property of `{wmi_class}`".format(
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
        connection = sampler.get_connection()
        tag_query_sampler = WMISampler(
            self.log,
            target_class, [target_property],
            filters=filters,
            **connection
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
                            name=tag_by.lower(), value=tag_value
                        )
                    )
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
                func = getattr(self, metric_type)
            except AttributeError:
                raise Exception(u"Invalid metric type: {0}".format(metric_type))

            func(metric_name, metric.value, metric.tags)

    def _get_instance_key(self, host, namespace, wmi_class):
        """
        Return an index key for a given instance. Usefull for caching.
        """
        return "{host}:{namespace}:{wmi_class}".format(
            host=host, namespace=namespace, wmi_class=wmi_class,
        )

    def _get_wmi_sampler(self, instance_key, wmi_class, properties, **kwargs):
        """
        Create and cache a WMISampler for the given (class, properties)
        """
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
