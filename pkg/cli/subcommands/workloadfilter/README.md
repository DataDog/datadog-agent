# Workload Filter CLI Commands

This package provides CLI commands for working with workload filters in the Datadog Agent.

## Commands

### `agent workloadfilter`

Prints the current workload filter status of a running agent, showing which filters are currently applied.

**Usage:**
```bash
agent workloadfilter
```

### `agent workloadfilter verify-cel`

Validates CEL (Common Expression Language) workload filter rules from a YAML or JSON file. This command is useful for:
- Testing filter configurations before deploying them
- Validating syntax and structure of CEL rules
- Debugging filter configurations

**Usage:**
```bash
agent workloadfilter verify-cel < your-config.yaml
# or
agent workloadfilter verify-cel < your-config.json
```

Or using file redirection:
```bash
cat your-config.yaml | agent workloadfilter verify-cel
cat your-config.json | agent workloadfilter verify-cel
```

## CEL Configuration Structure

The verify-cel command expects a YAML or JSON configuration file with the following structure:

**YAML format:**
```yaml
- products:
    - <product_name>  # metrics, logs, sbom, or global
  rules:
    <resource_type>:  # containers, pods, kube_services, kube_endpoints, or processes
      - <CEL_expression>
      - <CEL_expression>
```

**JSON format:**
```json
[
  {
    "products": ["<product_name>"],
    "rules": {
      "<resource_type>": [
        "<CEL_expression>",
        "<CEL_expression>"
      ]
    }
  }
]
```

## CEL Expression Syntax

CEL (Common Expression Language) is a powerful expression language that supports:

- **Comparison operators**: `==`, `!=`, `<`, `<=`, `>`, `>=`
- **Logical operators**: `&&` (AND), `||` (OR), `!` (NOT)
- **String functions**: 
  - `matches(pattern)` - Regular expression matching
  - `startsWith(prefix)` - String prefix matching
  - `endsWith(suffix)` - String suffix matching
  - `contains(substring)` - Substring matching
- **Map access**: `map["key"]` - Access map values by key
- **Parentheses**: `()` - Group expressions for precedence

## Validation Output

The verify-cel command provides detailed output during validation. It automatically detects the input format.

### Success Output

```
-> Validating CEL Configuration
    Loading YAML file...
✓ YAML loaded successfully (1 bundle(s))

-> Validating configuration structure...
✓ Configuration structure is valid

-> Compiling CEL rules...

  -> metrics
    Resource: container (2 rule(s))
      ✓ All rules compiled successfully

✅ All rules are valid!
```

### Error Output

When errors are found, the command provides detailed information:

```
-> Validating CEL Configuration
    Loading YAML file...
✓ YAML loaded successfully (1 bundle(s))

-> Validating configuration structure...
✓ Configuration structure is valid

-> Compiling CEL rules...

  -> metrics
    Resource: container (1 rule(s))
      ✗ Compilation failed: undeclared reference to 'nonexistent_field'
        Rule 1: container.nonexistent_field == "value"

✗ Validation failed - some rules have errors
Error: CEL compilation failed
```

For malformed input files:

```
-> Validating CEL Configuration
    Loading YAML file...
✗ Failed to unmarshal input (tried JSON and YAML)
Error: failed to parse input: ...
```

## See Also

- [Datadog Container Discovery Management Documentation](https://docs.datadoghq.com/containers/guide/container-discovery-management/)
- [CEL Language Specification](https://github.com/google/cel-spec)
- [Workload Filter Component Documentation](../../../../comp/core/workloadfilter/README.md)

