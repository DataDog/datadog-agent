# SECL Documentation

## Event types

| SECL Event | Type | Definition | Agent Version |
| ---------- | ---- | ---------- | ------------- |
{% for event_type in event_types %}
{% if event_type.name != "*" %}
| `{{ event_type.name }}` | {{ event_type.kind }} | {{ event_type.definition }} | {{ event_type.min_agent_version }} |
{% endif %}
{% endfor %}


{% for event_type in event_types %}
{% if event_type.name == "*" %}
{% set prefix = "*." %}
## Common to all event types
{% else %}
{% set prefix = "" %}
## Event `{{ event_type.name }}`
{% endif %}

| Property | Type | Definition |
| -------- | ---- | ---------- |
{% for property in event_type.properties %}
| `{{ prefix }}{{ property.name }}` | {{ property.type }} | {{ property.definition }} |
{% endfor %}

{% endfor %}
