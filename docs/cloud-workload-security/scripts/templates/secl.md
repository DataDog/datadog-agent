# SECL Documentation

## Event types

{% raw %}
{{< include-markdown "/content/en/security_platform/cloud_workload_security/_secl_event_types.md" >}}
{% endraw %}

{% for event_type in event_types %}
{% if event_type.name == "*" %}
{% set prefix = "*." %}
## Common to all event types
{% else %}
{% set prefix = "" %}
## Event `{{ event_type.name }}`

{{ event_type.definition }}
{% endif %}

| Property | Type | Definition |
| -------- | ---- | ---------- |
{% for property in event_type.properties %}
| `{{ prefix }}{{ property.name }}` | {{ property.datatype }} | {{ property.definition }} |
{% endfor %}

{% endfor %}
