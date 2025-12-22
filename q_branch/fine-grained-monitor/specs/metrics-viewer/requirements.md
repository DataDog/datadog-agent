# Metrics Viewer

## User Story

As an engineer investigating container behavior, I need to explore metrics
timeseries interactively so that I can identify patterns, anomalies, and
apply analytical studies across containers and metric types.

## Requirements

### REQ-MV-001: View Metrics Timeseries

WHEN user opens the viewer with a metrics file
THE SYSTEM SHALL display an interactive timeseries chart

WHEN no containers are selected
THE SYSTEM SHALL display an empty chart with instructions

**Rationale:** Engineers need visual representation of metrics to understand
container behavior over time. Raw numbers are difficult to interpret; charts
reveal patterns instantly.

---

### REQ-MV-002: Select Metrics to Display

WHEN user opens the metric selector
THE SYSTEM SHALL show all available metric types from the data file

WHEN user selects a metric
THE SYSTEM SHALL update the chart to display that metric for selected containers

WHEN user switches metric
THE SYSTEM SHALL preserve container selection and time range

**Rationale:** Different investigations require different signals. CPU usage
helps diagnose throttling; memory helps find leaks; IO helps find bottlenecks.
Users need to switch freely without losing context.

---

### REQ-MV-003: Filter Containers by Attributes

WHEN user applies a QoS class filter
THE SYSTEM SHALL show only containers matching that class

WHEN user applies a namespace filter
THE SYSTEM SHALL show only containers in that namespace

WHEN user combines multiple filters
THE SYSTEM SHALL show containers matching ALL applied filters

WHEN user clears filters
THE SYSTEM SHALL show all containers

WHEN user applies or changes any filter
THE SYSTEM SHALL preserve the current time range

**Rationale:** Large clusters have hundreds of containers. Quick filtering by
QoS class or namespace lets users focus on relevant workloads without scrolling
through long lists. Preserving the time window lets users compare different
container subsets at the same point in time.

---

### REQ-MV-004: Search and Select Containers

WHEN user types in the container search box
THE SYSTEM SHALL filter the container list to matching names

WHEN user selects containers from the list
THE SYSTEM SHALL add their timeseries to the chart

WHEN user deselects a container
THE SYSTEM SHALL remove its timeseries from the chart

WHEN user selects "Top N by average"
THE SYSTEM SHALL select the N containers with highest average value

WHEN user changes container selection (add, remove, or Top N)
THE SYSTEM SHALL preserve the current time range

**Rationale:** Users often know which container they want to investigate, or
want to focus on the busiest containers. Search and quick-select accelerate
this workflow. Preserving the time window lets users compare different
containers at the same moment—seeing how container A and B behaved during
the same incident.

---

### REQ-MV-005: Zoom and Pan Through Time

WHEN user drags on the chart to select a time region
THE SYSTEM SHALL zoom to show only that region

WHEN user scrolls on the chart
THE SYSTEM SHALL zoom in or out centered on cursor position

WHEN user drags while zoomed
THE SYSTEM SHALL pan the view in the drag direction

WHEN zoomed, WHEN user clicks reset
THE SYSTEM SHALL return to showing the full time range

**Rationale:** Metrics files can span hours or days. Users need to zoom into
specific incidents (seconds/minutes) while maintaining ability to see the full
picture.

---

### REQ-MV-006: Navigate with Range Overview

WHEN viewing a zoomed region
THE SYSTEM SHALL display a miniature overview of the full time range

WHEN user drags on the overview
THE SYSTEM SHALL pan the main chart to that time region

WHEN user resizes the selection on the overview
THE SYSTEM SHALL zoom the main chart to match

**Rationale:** When zoomed into detail, users lose context of where they are in
the overall timeline. The overview provides spatial context and fast navigation.

---

### REQ-MV-007: Detect Periodic Oscillations

WHEN user enables the Oscillation Study on displayed timeseries
THE SYSTEM SHALL analyze for periodic patterns

WHEN oscillation patterns are detected
THE SYSTEM SHALL highlight time regions where periodicity was found

WHEN user views oscillation results
THE SYSTEM SHALL display the detected period, confidence score, and amplitude

WHEN no periodic patterns meet detection threshold
THE SYSTEM SHALL indicate that no oscillations were found

**Rationale:** Periodic oscillations often indicate throttling, resource
contention, or scheduling issues. Manual detection requires tedious visual
scanning. Automated detection surfaces these patterns instantly, letting
engineers focus on root cause rather than pattern hunting.

---

### REQ-MV-008: Visualize Oscillation Patterns

WHEN oscillation regions are detected
THE SYSTEM SHALL overlay period markers on the chart within detected regions

WHEN user zooms or pans
THE SYSTEM SHALL update oscillation markers to remain aligned with the data

WHEN user hovers over an oscillation region
THE SYSTEM SHALL display a tooltip with period duration, confidence, and amplitude

WHEN user disables the Oscillation Study
THE SYSTEM SHALL remove all oscillation markers from the chart

**Rationale:** Seeing oscillation markers overlaid on raw data confirms the
detection and shows exactly when periodic behavior occurs. Engineers can
correlate oscillation timing with other events or container state changes.

---

### REQ-MV-009: Automatic Y-Axis Scaling

WHEN the visible time range changes (via zoom, pan, or reset)
THE SYSTEM SHALL automatically adjust Y-axis bounds to fit the visible data

WHEN displayed data changes (via metric, filter, or container selection)
THE SYSTEM SHALL automatically adjust Y-axis bounds to fit the new data

**Rationale:** Y-axis should always maximize use of chart space for whatever
data is currently visible. This reveals subtle variations that would be
compressed if the scale remained fixed to the original full-range bounds.

---
