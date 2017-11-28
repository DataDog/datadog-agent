# (C) Datadog, Inc. 2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

from google.protobuf.internal.decoder import _DecodeVarint32  # pylint: disable=E0611,E0401

from . import metrics_pb2


# Deprecated, please use the PrometheusCheck class
def parse_metric_family(buf):
    """
    Parse the binary buffer in input, searching for Prometheus messages
    of type MetricFamily [0] delimited by a varint32 [1].

    [0] https://github.com/prometheus/client_model/blob/086fe7ca28bde6cec2acd5223423c1475a362858/metrics.proto#L76-%20%20L81
    [1] https://developers.google.com/protocol-buffers/docs/reference/java/com/google/protobuf/AbstractMessageLite#writeDelimitedTo(java.io.OutputStream)
    """
    n = 0
    while n < len(buf):
        msg_len, new_pos = _DecodeVarint32(buf, n)
        n = new_pos
        msg_buf = buf[n:n+msg_len]
        n += msg_len

        message = metrics_pb2.MetricFamily()
        message.ParseFromString(msg_buf)
        yield message
