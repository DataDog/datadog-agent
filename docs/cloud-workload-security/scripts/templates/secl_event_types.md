| SECL Event | Type | Definition | Agent Version |
| ---------- | ---- | ---------- | ------------- |
{% for event_type in event_types %}
{% if event_type.name != "*" %}
| `{{ event_type.name }}` | {{ event_type.kind }} | {{ event_type.definition }} | {{ event_type.min_agent_version }} |
{% endif %}
{% endfor %}
