---
title: Cloud Workload Security (CWS) Events Formats
kind: documentation
description: JSON schema documentation of the CWS backend event
disable_edit: true
---

{{ warning_message }}

When activity matches a [Cloud Workload Security][1] (CWS) [Agent expression][2], a CWS log will be collected from the system containing all the relevant context about the activity.

This log is sent to Datadog, where it is analyzed. Based on analysis, CWS logs can trigger Security Signals or they can be stored as logs for audit, threat investigation purposes.

CWS logs have the following JSON schema:

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

[1]: /security/cloud_workload_security/
[2]: /security/cloud_workload_security/agent_expressions
