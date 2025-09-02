"""
Telemetry handler for sending events and metrics to Datadog from dynamic test components.
"""

import time
from abc import ABC, abstractmethod
from dataclasses import dataclass, field

from tasks.libs.common.datadog_api import create_count, create_gauge, send_event, send_metrics


@dataclass
class TelemetryEvent:
    """Represents an event to be sent to Datadog."""

    title: str
    text: str
    alert_type: str = "info"  # info, success, warning, error
    tags: list[str] = field(default_factory=list)
    timestamp: int | None = None
    host: str | None = None

    def __post_init__(self):
        if self.timestamp is None:
            self.timestamp = int(time.time())


@dataclass
class TelemetryMetric:
    """Represents a metric to be sent to Datadog."""

    name: str
    value: int | float
    metric_type: str = "gauge"  # gauge, count
    tags: list[str] = field(default_factory=list)
    timestamp: int | None = None
    host: str | None = None

    def __post_init__(self):
        if self.timestamp is None:
            self.timestamp = int(time.time())


class TelemetryHandler(ABC):
    """Abstract base class for telemetry handlers."""

    @abstractmethod
    def send_event(self, event: TelemetryEvent) -> bool:
        """Send an event to the telemetry backend.

        Args:
            event: The event to send

        Returns:
            True if successful, False otherwise
        """
        pass

    @abstractmethod
    def send_metric(self, metric: TelemetryMetric) -> bool:
        """Send a metric to the telemetry backend.

        Args:
            metric: The metric to send

        Returns:
            True if successful, False otherwise
        """
        pass

    @abstractmethod
    def count(self, name: str, value: int | float = 1, tags: list[str] | None = None) -> bool:
        """Increment a counter metric.

        Args:
            name: Metric name
            value: Value to increment by (default: 1)
            tags: Optional tags

        Returns:
            True if successful, False otherwise
        """
        pass

    @abstractmethod
    def gauge(self, name: str, value: int | float, tags: list[str] | None = None) -> bool:
        """Send a gauge metric.

        Args:
            name: Metric name
            value: Gauge value
            tags: Optional tags

        Returns:
            True if successful, False otherwise
        """
        pass


class DatadogTelemetryHandler(TelemetryHandler):
    """Datadog implementation of TelemetryHandler using existing datadog_api functions."""

    def __init__(self, default_tags: list[str] | None = None):
        """Initialize Datadog telemetry handler.

        Args:
            default_tags: Default tags to apply to all metrics/events
        """
        self.default_tags = default_tags or []

    def _merge_tags(self, tags: list[str] | None) -> list[str]:
        """Merge provided tags with default tags."""
        merged = self.default_tags.copy()
        if tags:
            merged.extend(tags)
        return merged

    def send_event(self, event: TelemetryEvent) -> bool:
        """Send an event to Datadog using existing API functions."""
        try:
            # Merge default tags with event tags
            merged_tags = self._merge_tags(event.tags)

            # Use existing send_event function
            send_event(title=event.title, text=event.text, tags=merged_tags)
            return True
        except Exception as e:
            print(f"Failed to send event '{event.title}': {e}")
            return False

    def send_metric(self, metric: TelemetryMetric) -> bool:
        """Send a metric to Datadog using existing API functions."""
        try:
            tags = self._merge_tags(metric.tags)

            if metric.metric_type == "gauge":
                series = [create_gauge(metric.name, metric.timestamp, metric.value, tags)]
            elif metric.metric_type == "count":
                series = [create_count(metric.name, metric.timestamp, metric.value, tags)]
            else:
                print(f"Unsupported metric type for API submission: {metric.metric_type}")
                return False

            send_metrics(series)
            return True
        except Exception as e:
            print(f"Failed to send metric '{metric.name}': {e}")
            return False

    def count(self, name: str, value: int | float = 1, tags: list[str] | None = None) -> bool:
        """Increment a counter metric."""
        metric = TelemetryMetric(name=name, value=value, metric_type="count", tags=tags or [])
        return self.send_metric(metric)

    def gauge(self, name: str, value: int | float, tags: list[str] | None = None) -> bool:
        """Send a gauge metric."""
        metric = TelemetryMetric(name=name, value=value, metric_type="gauge", tags=tags or [])
        return self.send_metric(metric)
