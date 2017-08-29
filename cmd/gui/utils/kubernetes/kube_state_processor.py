# (C) Datadog, Inc. 2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

NAMESPACE = 'kubernetes_state'

# message.type is the index in this array
# see: https://github.com/prometheus/client_model/blob/model-0.0.2/metrics.proto#L24-L28
METRIC_TYPES = ['counter', 'gauge']


class KubeStateProcessor:
    def __init__(self, kubernetes_check):
        self.kube_check = kubernetes_check
        self.log = self.kube_check.log
        self.gauge = self.kube_check.gauge
        self.service_check = kubernetes_check.service_check
        # Original camelcase keys have already been converted to lowercase.
        self.pod_phase_to_status = {
            'pending':   kubernetes_check.WARNING,
            'running':   kubernetes_check.OK,
            'succeeded': kubernetes_check.OK,
            'failed':    kubernetes_check.CRITICAL,
            # Rely on lookup default value
            # 'unknown':   AgentCheck.UNKNOWN
        }

        # these metrics will be extracted with all their labels and reported as-is with their corresponding metric name
        self.metric_to_gauge = {
            # message.metric: datadog metric name
            'kube_node_status_capacity_cpu_cores': NAMESPACE + '.node.cpu_capacity',
            'kube_node_status_capacity_memory_bytes': NAMESPACE + '.node.memory_capacity',
            'kube_node_status_capacity_pods': NAMESPACE + '.node.pods_capacity',
            'kube_node_status_allocatable_cpu_cores': NAMESPACE + '.node.cpu_allocatable',
            'kube_node_status_allocatable_memory_bytes': NAMESPACE + '.node.memory_allocatable',
            'kube_node_status_allocatable_pods': NAMESPACE + '.node.pods_allocatable',
            'kube_deployment_status_replicas_available': NAMESPACE + '.deployment.replicas_available',
            'kube_deployment_status_replicas_unavailable': NAMESPACE + '.deployment.replicas_unavailable',
            'kube_deployment_status_replicas_updated': NAMESPACE + '.deployment.replicas_updated',
            'kube_deployment_spec_replicas': NAMESPACE + '.deployment.replicas_desired',
            'kube_pod_container_resource_requests_cpu_cores': NAMESPACE + '.container.cpu_requested',
            'kube_pod_container_resource_requests_memory_bytes': NAMESPACE + '.container.memory_requested',
            'kube_pod_container_resource_limits_cpu_cores': NAMESPACE + '.container.cpu_limit',
            'kube_pod_container_resource_limits_memory_bytes': NAMESPACE + '.container.memory_limit',
            'kube_pod_container_status_restarts': NAMESPACE + '.container.restarts'
        }

    def process(self, message, **kwargs):
        """
        Handle a message according to the following flow:
            - search self.metric_to_gauge for a prometheus.metric <--> datadog.metric mapping
            - call check method with the same name as the metric
            - log some info if none of the above worked
        """
        try:
            if message.name in self.metric_to_gauge:
                self._submit_gauges(self.metric_to_gauge[message.name], message)
            else:
                getattr(self, message.name)(message, **kwargs)
        except AttributeError:
            self.log.debug("Unable to handle metric: {}".format(message.name))

    def _eval_metric_condition(self, metric):
        """
        Some metrics contains conditions, labels that have "condition" as name and "true", "false", or "unknown"
        as value. The metric value is expected to be a gauge equal to 0 or 1 in this case.

        This function acts as an helper to iterate and evaluate metrics containing conditions
        and returns a tuple containing the name of the condition and the boolean value.
        For example:

        metric {
          label {
            name: "condition"
            value: "true"
          }
          # other labels here
          gauge {
            value: 1.0
          }
        }

        would return `("true", True)`.

        Returns `None, None` if metric has no conditions.
        """
        val = bool(metric.gauge.value)
        for label in metric.label:
            if label.name == 'condition':
                return label.value, val

        return None, None

    def _extract_label_value(self, name, labels):
        """
        Search for `name` in labels name and returns
        corresponding value.
        Returns None if name was not found.
        """
        for label in labels:
            if label.name == name:
                return label.value
        return None

    def _submit_gauges(self, metric_name, message):
        """
        For each metric in the message, report it as a gauge with all labels as tags
        except if a labels dict is passed, in which case keys are label names we'll extract
        and corresponding values are tag names we'll use (eg: {'node': 'node'})
        """
        if message.type < len(METRIC_TYPES):
            for metric in message.metric:
                val = getattr(metric, METRIC_TYPES[message.type]).value
                tags = ['{}:{}'.format(label.name, label.value) for label in metric.label]
                self.gauge(metric_name, val, tags)
        else:
            self.log.error("Metric type %s unsupported for metric %s." % (message.type, message.name))

    # TODO: implement kube_pod_container_status_ready
    # TODO: implement kube_pod_container_status_running
    # TODO: implement kube_pod_container_status_terminated
    # TODO: implement kube_pod_container_status_waiting

    # TODO: implement kube_pod_info
    # TODO: implement kube_pod_status_ready
    # TODO: implement kube_pod_status_scheduled

    # Labels attached: namespace, pod, phase=Pending|Running|Succeeded|Failed|Unknown
    # The phase gets not passed through; rather, it becomes the service check suffix.
    def kube_pod_status_phase(self, message, **kwargs):
        """ Phase a pod is in. """
        check_basename = NAMESPACE + '.pod.phase.'
        for metric in message.metric:
            # The gauge value is always 1, no point in fetching it.
            phase = ''
            tags = []
            for label in metric.label:
                if label.name == 'phase':
                    phase = label.value.lower()
                else:
                    tags.append('{}:{}'.format(label.name, label.value))
            #TODO: add deployment/replicaset?
            status = self.pod_phase_to_status.get(phase, self.kube_check.UNKNOWN)
            self.service_check(check_basename + phase, status, tags=tags)

    def kube_node_status_ready(self, message, **kwargs):
        """ The ready status of a cluster node. """
        service_check_name = NAMESPACE + '.node.ready'
        for metric in message.metric:
            name, val = self._eval_metric_condition(metric)
            tags = ['node:{}'.format(self._extract_label_value("node", metric.label))]
            if name == 'true' and val:
                self.service_check(service_check_name, self.kube_check.OK, tags=tags)
            elif name == 'false' and val:
                self.service_check(service_check_name, self.kube_check.CRITICAL, tags=tags)
            elif name == 'unknown' and val:
                self.service_check(service_check_name, self.kube_check.UNKNOWN, tags=tags)

    def kube_node_status_out_of_disk(self, message, **kwargs):
        """ Whether the node is out of disk space. """
        service_check_name = NAMESPACE + '.node.out_of_disk'
        for metric in message.metric:
            name, val = self._eval_metric_condition(metric)
            tags = ['node:{}'.format(self._extract_label_value("node", metric.label))]
            if name == 'true' and val:
                self.service_check(service_check_name, self.kube_check.CRITICAL, tags=tags)
            elif name == 'false' and val:
                self.service_check(service_check_name, self.kube_check.OK, tags=tags)
            elif name == 'unknown' and val:
                self.service_check(service_check_name, self.kube_check.UNKNOWN, tags=tags)

    def kube_node_spec_unschedulable(self, message, **kwargs):
        """ Whether a node can schedule new pods. """
        metric_name = NAMESPACE + '.node.status'
        statuses = ('schedulable', 'unschedulable')
        if message.type < len(METRIC_TYPES):
            for metric in message.metric:
                tags = ['{}:{}'.format(label.name, label.value) for label in metric.label]
                status = statuses[int(getattr(metric, METRIC_TYPES[message.type]).value)]  # value can be 0 or 1
                tags.append('status:{}'.format(status))
                self.gauge(metric_name, 1, tags)  # metric value is always one, value is on the tags
        else:
            self.log.error("Metric type %s unsupported for metric %s" % (message.type, message.name))
