---
title: Cloud Workload Security (CWS) Events
kind: documentation
description: JSON schema documentation of the CWS backend event
disable_edit: true
---

After detecting suspicious activity matching the agent expressions, the Agent will send a log
to the backend containing a `CWS event`.

This event is used to build signals and its fields can be used to build filters in the web application.

The CWS event sent to the backend by the Agent respects the following JSON schema:

{% raw %}
{{< code-block lang="json" collapsible="true" filename="BACKEND_EVENT_JSON_SCHEMA" >}}
{% endraw %}
{{ event_schema }}
{% raw %}
{{< /code-block >}}
{% endraw %}

| Parameter | Type | Description |
| --------- | ---- | ----------- |
{% for param in parameters %}
| `{{ param.name }}` | {{ param.type }} | {{ param.description }} |
{% endfor %}

{% for def in definitions %}
## `{{ def.name }}`

{% raw %}
{{< code-block lang="json" collapsible="true" >}}
{% endraw %}
{{ def.schema }}
{% raw %}
{{< /code-block >}}
{% endraw %}

{% if def.descriptions %}
| Field | Description |
| ----- | ----------- |
{% for desc in def.descriptions %}
| `{{ desc.field_name }}` | {{ desc.description }} |
{% endfor %}
{% endif %}

{% if def.references %}
| References |
| ---------- |
{% for ref in def.references %}
| [{{ ref.name }}](#{{ ref.anchor }}) |
{% endfor %}
{% endif %}

{% endfor %}
