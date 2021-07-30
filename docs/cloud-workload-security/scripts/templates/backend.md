# Backend event Documentation

INTRO MSG:
```
BACKEND_EVENT_SCHEMA = {{ event_schema }}
```

| Parameter | Type | Description |
| --------- | ---- | ----------- |
{% for param in parameters %}
| `{{ param.name }}` | {{ param.type }} | {{ param.description }} |
{% endfor %}


{% for def in definitions %}

## `{{ def.name }}`

INTRO_MSG:
```
{{ def.schema }}
```

{% endfor %}
