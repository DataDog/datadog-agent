# CWS Event Documentation

The CWS event sent to the backend by the security agent respects the following schema:
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

{{< code-block lang="json" collapsible="true" >}}
{{ def.schema }}
{{< /code-block >}}

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
