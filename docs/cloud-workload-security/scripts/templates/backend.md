# Backend event Documentation

The CWS event sent to the backend by the security agent respects the following schema:
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

```
{{ def.schema }}
```

{% endfor %}
