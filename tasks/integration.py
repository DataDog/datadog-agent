"""
Invoke tasks for managing agent integrations (checks).

Commands:
  dda inv integration.spec-generate --check <name>   Parse conf.yaml → spec.yaml (single check)
  dda inv integration.spec-generate --all             Bootstrap spec.yaml for all checks missing one
  dda inv integration.spec-sync [--check <name>]     validate drift (default)
  dda inv integration.spec-sync --sync               apply changes from spec.yaml
  dda inv integration.create <name> --path <path>    Scaffold a new integration
"""

import difflib
import re
import textwrap
from pathlib import Path

import yaml
from invoke import task
from invoke.exceptions import Exit

# Repo-relative directories
ASSETS_DIR = Path("cmd/agent/dist/assets")
CONFD_DIR = Path("cmd/agent/dist/conf.d")
TEMPLATES_DIR = Path("tasks/integration_templates")

# Checks that predate the spec.yaml workflow; spec-sync skips them.
SPEC_SYNC_IGNORE = frozenset(
    {
        "network_path",
        "windows_certificate",
        "wincrashdetect",
        "systemd",
    }
)

# ---------------------------------------------------------------------------
# Template system
# ---------------------------------------------------------------------------

_template_cache: dict[str, list] = {}


def _load_template(name: str) -> list:
    """Load a template by path (e.g. 'instances/default') from TEMPLATES_DIR."""
    if name in _template_cache:
        return _template_cache[name]
    path = TEMPLATES_DIR / f"{name}.yaml"
    if not path.exists():
        raise Exit(f"Error: template '{name}' not found at {path}", code=1)
    with open(path) as f:
        options = yaml.safe_load(f) or []
    _template_cache[name] = options
    return options


def _resolve_options(options: list) -> list:
    """
    Expand any {'template': 'path/name'} entries in an options list into
    their constituent options, recursively.
    """
    resolved = []
    for opt in options:
        if "template" in opt and "name" not in opt:
            resolved.extend(_load_template(opt["template"]))
        else:
            resolved.append(opt)
    return resolved


def _common_params_for_template(template_name: str) -> set[str]:
    """Return the set of param names defined in a template file."""
    return {opt["name"] for opt in _load_template(template_name)}


# Params handled by CommonConfigure — derived from the instances/default template.
# Excluded from the Go Configuration struct.
def _common_instance_params() -> set[str]:
    return _common_params_for_template("instances/default")


# Spec type → Go type
_GO_TYPE_MAP = {
    "string": "string",
    "integer": "int",
    "number": "float64",
    "boolean": "bool",
    "object": "map[string]interface{}",
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_TYPE_MAP = {
    "boolean": "boolean",
    "string": "string",
    "integer": "integer",
    "number": "number",
    "list of strings": "array",
    "[]string": "array",
    "array of mappings": "array",
    "list of key:value elements": "array",
}


def _spec_type(raw_type: str) -> dict:
    """Convert @param type string to spec.yaml value dict."""
    normalized = raw_type.lower().strip()
    spec_type = _TYPE_MAP.get(normalized, "string")
    value: dict = {"type": spec_type}
    if spec_type == "array":
        value["items"] = {"type": "string"}
    return value


def _render_param_comment(name: str, param_type: str, required: bool, default, description: str, example) -> str:
    """Render the ## @param comment block for a conf.yaml entry."""
    req_str = "required" if required else "optional"
    header = f"## @param {name} - {param_type} - {req_str}"
    if default is not None:
        default_str = str(default).lower() if isinstance(default, bool) else str(default)
        header += f" - default: {default_str}"
    lines = [header]
    for desc_line in description.rstrip().splitlines():
        lines.append(f"## {desc_line}" if desc_line.strip() else "##")
    lines.append("#")

    # Format example value for YAML
    if example is None:
        example_str = f"<{name.upper()}>"
    elif isinstance(example, list):
        example_str = "\n".join(f"#   - {item}" for item in example)
        if required:
            return "\n".join(lines) + f"\n{name}:\n" + "\n".join(f"  - {item}" for item in example) + "\n"
        return "\n".join(lines) + f"\n# {name}:\n" + example_str + "\n"
    elif isinstance(example, dict):
        import yaml as _yaml

        example_yaml = _yaml.dump(example, default_flow_style=False).rstrip()
        indented = "\n".join(f"#   {line}" for line in example_yaml.splitlines())
        if required:
            return "\n".join(lines) + f"\n{name}:\n  # ...\n"
        return "\n".join(lines) + f"\n# {name}:\n{indented}\n"
    else:
        example_str = str(example).lower() if isinstance(example, bool) else str(example)

    if required:
        return "\n".join(lines) + f"\n{name}: {example_str}\n"
    return "\n".join(lines) + f"\n# {name}: {example_str}\n"


def _option_to_param_type(value: dict) -> str:
    """Convert spec value dict back to human-readable @param type."""
    spec_type = value.get("type", "string")
    if spec_type == "array":
        return "list of strings"
    return spec_type


def _render_object_section(name: str, description: str, required: bool, properties: dict) -> str:
    """
    Render an object option whose sub-properties each have a description as an
    indented block of ## @param entries (no @param on the parent itself).

    Example output (before outer indentation):
        ## <description>
        #
        # <name>:

          ## @param <prop> - <type> - optional
          ## <prop description>
          #
          # <prop>: <example>
    """
    lines = []
    for desc_line in description.rstrip().splitlines():
        lines.append(f"## {desc_line}" if desc_line.strip() else "##")
    lines.append("#")
    prefix = "" if required else "# "
    lines.append(f"{prefix}{name}:")

    for prop_name, prop in properties.items():
        prop_type = prop.get("type", "string")
        param_type = "list of strings" if prop_type == "array" else prop_type
        prop_required = prop.get("required", False)
        default = prop.get("default")
        example = prop.get("example")
        prop_desc = prop.get("description", "").strip()
        block = _render_param_comment(prop_name, param_type, prop_required, default, prop_desc, example)
        lines.append(textwrap.indent(block, "  "))

    return "\n".join(lines)


def _render_object_mapping(name: str, description: str, required: bool, properties: dict) -> str:
    """
    Render an object option whose sub-properties have no descriptions as a flat
    ## @param block with commented YAML showing the structure.

    Example output (before outer indentation):
        ## @param <name> - mapping - optional
        ## <description>
        #
        # <name>:
        #   <prop1>:
        #   - <example_item>
    """
    req_str = "required" if required else "optional"
    lines = [f"## @param {name} - mapping - {req_str}"]
    for desc_line in description.rstrip().splitlines():
        lines.append(f"## {desc_line}" if desc_line.strip() else "##")
    lines.append("#")
    lines.append(f"# {name}:")
    for prop_name, prop in properties.items():
        prop_type = prop.get("type", "string")
        example = prop.get("example")
        if prop_type == "array" and isinstance(example, list):
            lines.append(f"#   {prop_name}:")
            for item in example:
                lines.append(f"#   - {item}")
        else:
            example_str = str(example) if example is not None else f"<{prop_name.upper()}>"
            lines.append(f"#   {prop_name}: {example_str}")
    return "\n".join(lines)


def _render_opt_block(opt: dict) -> str:
    """Dispatch to the appropriate renderer for a single option entry."""
    name = opt["name"]
    value = opt.get("value", {})
    description = opt.get("description", "").strip()
    required = opt.get("required", False)

    properties = value.get("properties")
    if properties and value.get("type") == "object":
        has_descriptions = any(p.get("description") for p in properties.values())
        if has_descriptions:
            return _render_object_section(name, description, required, properties)
        return _render_object_mapping(name, description, required, properties)

    default = value.get("default")
    example = value.get("example")
    param_type = _option_to_param_type(value)
    return _render_param_comment(name, param_type, required, default, description, example)


def _generate_conf_yaml(check_name: str, spec: dict) -> str:
    """Render a conf.yaml.example string from a parsed spec dict."""
    lines = [
        "## All options defined here are available to all instances.",
        "##",
        "## WARNING: To avoid any issues with parameter overrides, do not copy/paste this file.",
        "## Instead edit your configuration file to only include the required parameters.",
        "",
    ]

    common_params = _common_instance_params()
    for file_entry in spec.get("files", []):
        for template_block in file_entry.get("options", []):
            template = template_block.get("template", "")
            opts = _resolve_options(template_block.get("options", []))
            if not opts:
                continue
            lines.append(f"{template}:")
            lines.append("")

            if template == "instances":
                check_opts = [o for o in opts if o.get("name") not in common_params]
                common_opts = [o for o in opts if o.get("name") in common_params]
                lines.append("  -")
                for opt in check_opts + common_opts:
                    lines.append(textwrap.indent(_render_opt_block(opt), "    "))
            else:
                for opt in opts:
                    lines.append(_render_opt_block(opt))
            lines.append("")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Task: spec-generate
# ---------------------------------------------------------------------------


@task(
    help={
        "check": "Name of the check (e.g. 'ntp'). Mutually exclusive with --all.",
        "all_checks": "Generate spec.yaml for every check that has a conf.yaml.example but no spec.yaml yet.",
        "overwrite": "Overwrite existing spec.yaml if it exists (default: False).",
    }
)
def spec_generate(_, check=None, all_checks=False, overwrite=False):
    """
    Parse @param annotations from an existing conf.yaml and generate spec.yaml.

    Use --check <name> for a single check, or --all to bootstrap every check that
    has a conf.yaml.example but no spec.yaml yet.

    The generated spec.yaml is written to cmd/agent/dist/assets/<check>/spec.yaml.
    Edit it after generation to add descriptions, defaults, and fleet_configurable flags.
    """
    if check and all_checks:
        raise Exit("Error: --check and --all are mutually exclusive.", code=1)
    if not check and not all_checks:
        raise Exit("Error: provide --check <name> or --all.", code=1)

    if all_checks:
        _spec_generate_all(overwrite)
        return

    _spec_generate_one(check, overwrite)


def _spec_generate_one(check: str, overwrite: bool):
    """Generate spec.yaml for a single check."""
    check_dir = CONFD_DIR / f"{check}.d"
    conf_path = None
    for candidate in ("conf.yaml.default", "conf.yaml.example"):
        p = check_dir / candidate
        if p.exists():
            conf_path = p
            break

    if conf_path is None:
        raise Exit(
            f"Error: No conf.yaml.default or conf.yaml.example found in {check_dir}.\n"
            "Make sure the check directory exists.",
            code=1,
        )

    out_dir = ASSETS_DIR / check
    out_path = out_dir / "spec.yaml"

    if out_path.exists() and not overwrite:
        raise Exit(
            f"Error: {out_path} already exists. Use --overwrite to replace it.",
            code=1,
        )

    print(f"Parsing {conf_path} ...")
    options_by_template = _parse_conf_yaml(conf_path)
    spec = _build_spec(check, options_by_template)

    out_dir.mkdir(parents=True, exist_ok=True)
    with open(out_path, "w") as f:
        yaml.dump(spec, f, default_flow_style=False, sort_keys=False, allow_unicode=True)

    print(f"Generated {out_path}")
    print("Review the file and set fleet_configurable: true for options you want to manage via Fleet.")


def _spec_generate_all(overwrite: bool):
    """Generate spec.yaml for every check that has a conf.yaml.example but no spec.yaml."""
    generated = []
    skipped = []

    for conf_path in sorted(CONFD_DIR.glob("*.d/conf.yaml.example")):
        check_name = conf_path.parent.name.removesuffix(".d")
        out_path = ASSETS_DIR / check_name / "spec.yaml"

        if out_path.exists() and not overwrite:
            skipped.append(check_name)
            continue

        options_by_template = _parse_conf_yaml(conf_path)
        spec = _build_spec(check_name, options_by_template)

        out_path.parent.mkdir(parents=True, exist_ok=True)
        with open(out_path, "w") as f:
            yaml.dump(spec, f, default_flow_style=False, sort_keys=False, allow_unicode=True)

        generated.append(out_path)

    if generated:
        print(f"Generated {len(generated)} spec.yaml file(s):")
        for p in generated:
            print(f"  {p}")
        print("\nReview each file and set fleet_configurable: true where appropriate.")

    if skipped:
        print(f"\nSkipped {len(skipped)} check(s) with existing spec.yaml (use --overwrite to replace):")
        for name in skipped:
            print(f"  {name}")

    if not generated and not skipped:
        print(f"No conf.yaml.example files found under {CONFD_DIR}.")


def _parse_conf_yaml(conf_path: Path) -> dict:
    """
    Parse @param annotation blocks from a conf.yaml file.
    Returns {'init_config': [...], 'instances': [...]}
    """
    content = conf_path.read_text()
    lines = content.splitlines()

    options_by_template = {"init_config": [], "instances": []}
    current_template = "instances"  # default section

    # State machine for parsing @param blocks
    i = 0
    pending_desc_lines: list[str] = []  # ## lines buffered before a section-style object

    while i < len(lines):
        raw_line = lines[i]
        stripped = raw_line.strip()

        # Detect section headers
        if re.match(r'^init_config\s*:', stripped):
            current_template = "init_config"
            pending_desc_lines = []
            i += 1
            continue
        if re.match(r'^instances\s*:', stripped):
            current_template = "instances"
            pending_desc_lines = []
            i += 1
            continue

        # Detect section-style object: "# name:" not preceded by @param, followed by indented @param sub-blocks
        section_match = re.match(r'^#\s+(\w+)\s*:\s*$', stripped)
        if section_match:
            # Peek ahead: find first non-empty/non-separator stripped line
            j = i + 1
            while j < len(lines) and lines[j].strip() in ('', '#'):
                j += 1
            if j < len(lines) and re.match(r'^##\s+@param\s+', lines[j].strip()):
                parent_name = section_match.group(1)
                description = "\n".join(pending_desc_lines).strip()
                pending_desc_lines = []
                base_indent = len(raw_line) - len(raw_line.lstrip())
                properties: dict = {}
                i = j

                while i < len(lines):
                    raw = lines[i]
                    s = raw.strip()
                    if not s or s == '#':
                        i += 1
                        continue
                    curr_indent = len(raw) - len(raw.lstrip())
                    if curr_indent <= base_indent:
                        break
                    sub_param = re.match(r'^##\s+@param\s+(\S+)\s+-\s+(.+)', s)
                    if sub_param:
                        prop_name = sub_param.group(1)
                        rest = sub_param.group(2)
                        parts = [p.strip() for p in rest.split(" - ")]
                        raw_type = parts[0] if parts else "string"
                        prop_required = False
                        prop_default = None
                        for part in parts[1:]:
                            if part.lower() == "required":
                                prop_required = True
                            elif part.lower() == "optional":
                                prop_required = False
                            elif part.lower().startswith("default:"):
                                prop_default = _parse_scalar(part[len("default:") :].strip())
                        i += 1
                        prop_desc_lines: list[str] = []
                        while i < len(lines):
                            s2 = lines[i].strip()
                            if s2.startswith("## ") or s2 == "##":
                                text = s2[3:] if s2.startswith("## ") else ""
                                if text.startswith("@param"):
                                    break
                                prop_desc_lines.append(text)
                                i += 1
                            else:
                                break
                        prop_example = None
                        k = i
                        while k < len(lines) and lines[k].strip() in ('#', ''):
                            k += 1
                        if k < len(lines):
                            ex_match = re.match(r'^[\s#]*#\s+' + re.escape(prop_name) + r'\s*:\s*(.*)', lines[k])
                            if ex_match:
                                raw_ex = ex_match.group(1).strip()
                                if raw_ex:
                                    prop_example = _parse_scalar(raw_ex)
                                items = []
                                m = k + 1
                                while m < len(lines):
                                    item_match = re.match(r'^[\s#]*#\s+-\s+(.*)', lines[m])
                                    if item_match:
                                        items.append(item_match.group(1).strip())
                                        m += 1
                                    else:
                                        break
                                if items:
                                    prop_example = items
                        prop_value = _spec_type(raw_type)
                        if prop_example is not None:
                            prop_value["example"] = prop_example
                        else:
                            prop_value["example"] = f"<{prop_name.upper()}>"
                        if prop_default is not None:
                            prop_value["default"] = prop_default
                        properties[prop_name] = {
                            "description": "\n".join(prop_desc_lines).strip() + "\n",
                            "required": prop_required,
                            **prop_value,
                        }
                    else:
                        i += 1

                options_by_template[current_template].append(
                    {
                        "name": parent_name,
                        "fleet_configurable": False,
                        "value": {"type": "object", "properties": properties},
                        "description": description + "\n",
                        "required": False,
                    }
                )
                continue

        # Detect @param annotation
        param_match = re.match(r'^##\s+@param\s+(\S+)\s+-\s+(.+)', stripped)
        if param_match:
            pending_desc_lines = []
            param_name = param_match.group(1)
            rest = param_match.group(2)

            # Parse type and required/optional from rest
            parts = [p.strip() for p in rest.split(" - ")]
            raw_type = parts[0] if parts else "string"
            is_required = False
            default_val = None

            for part in parts[1:]:
                if part.lower() == "required":
                    is_required = True
                elif part.lower() == "optional":
                    is_required = False
                elif part.lower().startswith("default:"):
                    default_str = part[len("default:") :].strip()
                    default_val = _parse_scalar(default_str)

            # Collect description lines (## lines after @param, until blank ## or non-## line)
            i += 1
            desc_lines = []
            while i < len(lines):
                s = lines[i].strip()
                if s.startswith("## ") or s == "##":
                    text = s[3:] if s.startswith("## ") else ""
                    # Stop if we hit another @param
                    if text.startswith("@param"):
                        break
                    desc_lines.append(text)
                    i += 1
                else:
                    break

            # Look ahead for the commented example value: `# param_name: value` or `# param_name:`
            example_val = None
            j = i
            while j < len(lines) and lines[j].strip() in ("#", ""):
                j += 1
            if j < len(lines):
                example_match = re.match(r'^[\s#]*#\s+' + re.escape(param_name) + r'\s*:\s*(.*)', lines[j])
                if example_match:
                    raw_example = example_match.group(1).strip()
                    if raw_example:
                        example_val = _parse_scalar(raw_example)
                    # Check for list example (subsequent `#   - item` lines)
                    items = []
                    k = j + 1
                    while k < len(lines):
                        item_match = re.match(r'^[\s#]*#\s+-\s+(.*)', lines[k])
                        if item_match:
                            items.append(item_match.group(1).strip())
                            k += 1
                        else:
                            break
                    if items:
                        example_val = items

            value_dict = _spec_type(raw_type)
            if example_val is not None:
                value_dict["example"] = example_val
            else:
                value_dict["example"] = f"<{param_name.upper()}>"
            if default_val is not None:
                value_dict["default"] = default_val

            opt = {
                "name": param_name,
                "fleet_configurable": False,
                "value": value_dict,
                "description": "\n".join(desc_lines).strip() + "\n",
                "required": is_required,
            }
            options_by_template[current_template].append(opt)
            continue

        # Accumulate ## description lines (not @param) into buffer for section-style objects
        if stripped.startswith("## ") or stripped == "##":
            text = stripped[3:] if stripped.startswith("## ") else ""
            pending_desc_lines.append(text)
        elif stripped not in ('#', ''):
            # Non-comment, non-separator line resets the description buffer
            pending_desc_lines = []

        i += 1

    return options_by_template


def _parse_scalar(s: str):
    """Try to parse a scalar YAML value from a string."""
    try:
        return yaml.safe_load(s)
    except Exception:
        return s


def _build_spec(check_name: str, options_by_template: dict) -> dict:
    """Build the full spec.yaml structure."""
    template_blocks = []
    for tmpl in ("init_config", "instances"):
        opts = options_by_template.get(tmpl, [])
        if opts:
            template_blocks.append({"template": tmpl, "options": opts})

    return {
        "name": check_name,
        "fleet_configurable": False,
        "version": "1.0.0",
        "files": [
            {
                "name": f"{check_name}.yaml",
                "options": template_blocks,
            }
        ],
    }


# ---------------------------------------------------------------------------
# Go struct sync helpers
# ---------------------------------------------------------------------------


def _spec_value_to_go_type(value: dict, field_name: str = "") -> str:
    """Convert a spec value dict to a Go type string."""
    spec_type = value.get("type", "string")
    if spec_type == "array":
        item_type = value.get("items", {}).get("type", "string")
        return f"[]{_GO_TYPE_MAP.get(item_type, 'interface{}')}"
    if spec_type == "object" and field_name:
        properties = value.get("properties", {})
        has_descriptions = any(p.get("description") for p in properties.values())
        if has_descriptions:
            return _snake_to_pascal(field_name)
    return _GO_TYPE_MAP.get(spec_type, "interface{}")


def _snake_to_pascal(name: str) -> str:
    return "".join(part.capitalize() for part in name.split("_"))


def _to_unexported(name: str) -> str:
    """Lowercase the first character of a PascalCase name (Go unexported)."""
    return name[0].lower() + name[1:] if name else name


def _build_config_struct_fields(spec: dict) -> tuple[list[str], dict[str, tuple[str, list[str]]]]:
    """
    Return Go struct field lines for all non-common instance options in the spec,
    plus a dict of helper struct types needed for section-style object fields.

    Returns:
        fields         – lines for the Configuration struct body
        helper_structs – {TypeName: (description, [field lines])} for nested struct types
    """
    fields: list[str] = []
    helper_structs: dict[str, tuple[str, list[str]]] = {}
    for file_entry in spec.get("files", []):
        for block in file_entry.get("options", []):
            if block.get("template") != "instances":
                continue
            for opt in _resolve_options(block.get("options", [])):
                name = opt["name"]
                if name in _common_instance_params():
                    continue
                value = opt.get("value", {})
                properties = value.get("properties", {})
                has_desc = any(p.get("description") for p in properties.values())
                if value.get("type") == "object" and properties and has_desc:
                    type_name = _snake_to_pascal(name)
                    fields.append(f'\t{type_name} {type_name} `yaml:"{name}"`')
                    sub_fields = []
                    for prop_name, prop in properties.items():
                        go_type = _spec_value_to_go_type(prop)
                        sub_fields.append(f'\t{_snake_to_pascal(prop_name)} {go_type} `yaml:"{prop_name}"`')
                    desc = opt.get("description", "").strip()
                    helper_structs[type_name] = (desc, sub_fields)
                else:
                    go_type = _spec_value_to_go_type(value, name)
                    field_name = _snake_to_pascal(name)
                    fields.append(f'\t{field_name} {go_type} `yaml:"{name}"`')
    return fields, helper_structs


def _find_check_go_file(check_name: str) -> Path | None:
    """Search pkg/collector/corechecks for <check_name>/<check_name>.go."""
    for candidate in Path("pkg/collector/corechecks").rglob(f"{check_name}/{check_name}.go"):
        return candidate
    return None


def _insert_fields_into_body(existing_body: str, new_field_lines: list[str]) -> str:
    """
    Append new_field_lines into existing_body just before the trailing blank lines.
    Returns the updated body string (preserves surrounding whitespace).
    """
    lines = existing_body.splitlines()
    # Find insertion point: just after the last non-empty line
    insert_at = len(lines)
    while insert_at > 0 and not lines[insert_at - 1].strip():
        insert_at -= 1
    for line in new_field_lines:
        lines.insert(insert_at, line)
        insert_at += 1
    result = "\n".join(lines)
    if result and not result.endswith("\n"):
        result += "\n"
    return result


def _sync_config_struct(
    go_file: Path,
    fields: list[str],
    helper_structs: dict[str, tuple[str, list[str]]],
) -> bool:
    """
    Additively sync Go struct fields derived from spec.yaml into go_file.

    Only adds fields whose yaml tag is not already present — never removes or
    overwrites existing field lines (preserving hand-written tags like json:,
    required:, nullable:, etc.).

    For helper structs (section-style objects):
    - If the struct already exists: adds any missing fields additively.
    - If it doesn't exist: inserts the full struct after type Configuration struct.

    Returns True if the file was changed.
    """
    content = go_file.read_text()
    updated = content

    # Detect existing casing for each helper struct (exported vs unexported).
    resolved_names: dict[str, str] = {}
    for type_name in helper_structs:
        unexported = _to_unexported(type_name)
        existing = re.search(rf'type ({re.escape(type_name)}|{re.escape(unexported)}) struct \{{', content)
        resolved_names[type_name] = existing.group(1) if existing else type_name

    # Rewrite field type references to match detected casing.
    resolved_fields = []
    for f in fields:
        line = f
        for exported_name, used_name in resolved_names.items():
            line = line.replace(f" {exported_name} ", f" {used_name} ")
        resolved_fields.append(line)

    # --- Main struct: additive update ---
    m = re.search(r'type Configuration struct \{([^}]*)\}', updated, re.DOTALL)
    if not m:
        return False  # struct not found in this file, nothing to do

    existing_body = m.group(1)
    existing_tags = {t.group(1) for t in re.finditer(r'`yaml:"([^"]+)"', existing_body)}
    new_fields = [f for f in resolved_fields if _yaml_tag(f) and _yaml_tag(f) not in existing_tags]

    if new_fields:
        new_body = _insert_fields_into_body(existing_body, new_fields)
        new_struct = f"type Configuration struct {{{new_body}}}"
        updated = updated[: m.start()] + new_struct + updated[m.end() :]

    # --- Helper structs: additive update or insert ---
    new_helper_blocks = []
    for type_name, (desc, sub_fields) in helper_structs.items():
        used_name = resolved_names[type_name]
        unexported = _to_unexported(type_name)

        hm = re.search(
            rf'type ({re.escape(type_name)}|{re.escape(unexported)}) struct \{{([^}}]*)\}}',
            updated,
            re.DOTALL,
        )
        if hm:
            # Struct exists — add only missing sub-fields
            existing_helper_body = hm.group(2)
            existing_helper_tags = {t.group(1) for t in re.finditer(r'`yaml:"([^"]+)"', existing_helper_body)}
            new_sub = [f for f in sub_fields if _yaml_tag(f) and _yaml_tag(f) not in existing_helper_tags]
            if new_sub:
                new_helper_body = _insert_fields_into_body(existing_helper_body, new_sub)
                # existing_helper_body already starts with \n — don't add another one
                new_helper_struct = f"type {hm.group(1)} struct {{{new_helper_body}}}"
                updated = updated[: hm.start()] + new_helper_struct + updated[hm.end() :]
        else:
            # Struct doesn't exist — queue for insertion before the main struct
            sub_body = ("\n".join(sub_fields) + "\n") if sub_fields else ""
            helper_struct = f"type {used_name} struct {{\n{sub_body}}}"
            if desc:
                normalized = desc[0].lower() + desc[1:]
                if not normalized.endswith("."):
                    normalized += "."
                comment = f"// {used_name} holds {normalized}"
            else:
                comment = f"// {used_name} holds nested configuration."
            new_helper_blocks.append(f"{comment}\n{helper_struct}")

    # Insert any brand-new helper structs after the main struct's closing brace
    if new_helper_blocks:
        main_m = re.search(r'type Configuration struct \{[^}]*\}', updated, re.DOTALL)
        if main_m:
            joined = "\n\n" + "\n\n".join(new_helper_blocks)
            updated = updated[: main_m.end()] + joined + updated[main_m.end() :]

    if updated == content:
        return False
    go_file.write_text(updated)
    return True


def _yaml_tag(field_line: str) -> str | None:
    """Extract the yaml tag value from a Go struct field line, or None."""
    m = re.search(r'`yaml:"([^"]+)"', field_line)
    return m.group(1) if m else None


# ---------------------------------------------------------------------------
# Task: spec-sync
# ---------------------------------------------------------------------------


@task(
    help={
        "check": "Validate/sync only this check (optional). Runs against all checks if not set.",
        "sync": "Write files instead of just validating.",
    }
)
def spec_sync(_, check=None, sync=False):
    """
    Validate that conf.yaml.example and Configuration structs match spec.yaml.

    By default, reports any drift without writing files. Pass --sync to apply changes.
    """
    if check:
        spec_paths = [ASSETS_DIR / check / "spec.yaml"]
    else:
        spec_paths = sorted(ASSETS_DIR.glob("*/spec.yaml"))

    if not spec_paths:
        raise Exit("No spec.yaml files found.", code=1)

    errors = []
    synced = 0

    for spec_path in spec_paths:
        if not spec_path.exists():
            errors.append(f"spec.yaml not found: {spec_path}")
            continue

        check_name = spec_path.parent.name
        if check_name in SPEC_SYNC_IGNORE:
            print(f"SKIP (legacy): {check_name}")
            continue

        with open(spec_path) as f:
            spec = yaml.safe_load(f)

        generated = _generate_conf_yaml(check_name, spec)

        out_dir = CONFD_DIR / f"{check_name}.d"
        out_path = out_dir / "conf.yaml.example"

        fields, helper_structs = _build_config_struct_fields(spec)
        go_file = _find_check_go_file(check_name)

        if sync:
            out_dir.mkdir(parents=True, exist_ok=True)
            out_path.write_text(generated)
            print(f"Wrote {out_path}")

            if go_file:
                if _sync_config_struct(go_file, fields, helper_structs):
                    print(f"Updated Configuration struct in {go_file}")
                else:
                    print(f"Configuration struct already up to date in {go_file}")
            else:
                print(
                    f"Note: no Go file found for '{check_name}' under pkg/collector/corechecks — skipping struct sync."
                )

            synced += 1
        else:
            if out_path.exists():
                existing = out_path.read_text()
                if existing != generated:
                    diff = "".join(
                        difflib.unified_diff(
                            existing.splitlines(keepends=True),
                            generated.splitlines(keepends=True),
                            fromfile=str(out_path),
                            tofile="spec.yaml (generated)",
                        )
                    )
                    errors.append(
                        f"DRIFT: {out_path}\n{diff}\n"
                        f"  → Run `dda inv integration.spec-sync --check {check_name} --sync` to fix."
                    )
                else:
                    print(f"OK (conf): {check_name}")
            else:
                errors.append(
                    f"MISSING: {out_path} does not exist.\n"
                    f"  → Run `dda inv integration.spec-sync --check {check_name} --sync` to create it."
                )

            if go_file:
                content = go_file.read_text()

                # The struct field may reference the helper type as exported or unexported.
                # Build an alternative body that uses unexported type names so we accept either.
                alt_fields = []
                for f in fields:
                    alt = f
                    for tn in helper_structs:
                        alt = alt.replace(f" {tn} ", f" {_to_unexported(tn)} ")
                    alt_fields.append(alt)

                body = ("\n".join(fields) + "\n") if fields else ""
                alt_body = ("\n".join(alt_fields) + "\n") if alt_fields else ""
                expected_struct = f"type Configuration struct {{\n{body}}}"
                alt_struct = f"type Configuration struct {{\n{alt_body}}}"
                struct_ok = expected_struct in content or alt_struct in content

                # Also check helper structs (accept exported or unexported type name)
                helpers_ok = True
                for type_name, (_desc, sub_fields) in helper_structs.items():
                    sub_body = ("\n".join(sub_fields) + "\n") if sub_fields else ""
                    exported_helper = f"type {type_name} struct {{\n{sub_body}}}"
                    unexported_helper = f"type {_to_unexported(type_name)} struct {{\n{sub_body}}}"
                    if exported_helper not in content and unexported_helper not in content:
                        helpers_ok = False
                        errors.append(
                            f"DRIFT: helper struct {type_name} in {go_file}\n"
                            f"  → Run `dda inv integration.spec-sync --check {check_name} --sync` to fix."
                        )

                if not struct_ok:
                    actual_match = re.search(r'type Configuration struct \{([^}]*)\}', content, re.DOTALL)
                    actual_body = actual_match.group(1) if actual_match else ""

                    expected_names = {m.group(1) for m in re.finditer(r'`yaml:"([^"]+)"`', body)}
                    actual_names = {m.group(1) for m in re.finditer(r'`yaml:"([^"]+)"`', actual_body)}

                    only_in_struct = actual_names - expected_names
                    only_in_spec = expected_names - actual_names

                    if only_in_struct:
                        errors.append(
                            f"SPEC OUT OF DATE: {go_file}\n"
                            f"  Field(s) in Go struct but not in spec.yaml: {', '.join(sorted(only_in_struct))}\n"
                            f"  → Add them to {ASSETS_DIR / check_name / 'spec.yaml'}"
                        )
                    if only_in_spec:
                        actual_struct = f"type Configuration struct {{{actual_body}}}"
                        diff = "".join(
                            difflib.unified_diff(
                                actual_struct.splitlines(keepends=True),
                                expected_struct.splitlines(keepends=True),
                                fromfile=str(go_file),
                                tofile="spec.yaml (generated)",
                            )
                        )
                        errors.append(
                            f"DRIFT: Configuration struct in {go_file}\n{diff}\n"
                            f"  → Run `dda inv integration.spec-sync --check {check_name} --sync` to fix."
                        )
                    if not only_in_struct and not only_in_spec and helpers_ok:
                        print(f"OK (struct): {check_name}")
                elif helpers_ok:
                    print(f"OK (struct): {check_name}")

    if errors:
        sep = "\n" + "-" * 60 + "\n"
        print(f"\nintegration.spec-sync found {len(errors)} issue(s):\n{sep}")
        for i, err in enumerate(errors, 1):
            print(f"{i}. {err}")
            if i < len(errors):
                print(sep)
        raise Exit(code=1)

    if sync:
        print(f"\nSynced {synced} integration(s).")


# ---------------------------------------------------------------------------
# Task: create
# ---------------------------------------------------------------------------

_CORECHECKS_GO = Path("pkg/commonchecks/corechecks.go")
_AGENT_PY = Path("tasks/agent.py")
_GO_MODULE_PREFIX = "github.com/DataDog/datadog-agent"


def _register_in_corechecks(name: str, go_pkg_path: str) -> bool:
    """
    Add an import and RegisterCheck call for `name` to corechecks.go.
    go_pkg_path is the repo-relative path to the package (e.g. 'pkg/collector/corechecks/my_check').
    Returns True if the file was changed.
    """
    content = _CORECHECKS_GO.read_text()
    import_path = f'"{_GO_MODULE_PREFIX}/{go_pkg_path}"'

    if import_path in content:
        return False  # already registered

    # Insert import before the closing ) of the import block
    content = content.replace(
        'corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"',
        f'corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"\n\t{import_path}',
    )

    # Insert RegisterCheck before registerSystemProbeChecks
    content = content.replace(
        '\n\n\tregisterSystemProbeChecks(tagger)',
        f'\n\tcorecheckLoader.RegisterCheck({name}.CheckName, {name}.Factory())\n\n\tregisterSystemProbeChecks(tagger)',
    )

    _CORECHECKS_GO.write_text(content)
    return True


def _register_in_agent_py(name: str, windows_only: bool) -> bool:
    """
    Add the check name to AGENT_CORECHECKS or WINDOWS_CORECHECKS in tasks/agent.py,
    inserted in alphabetical order.
    Returns True if the file was changed.
    """
    content = _AGENT_PY.read_text()
    list_name = "WINDOWS_CORECHECKS" if windows_only else "AGENT_CORECHECKS"

    # Check already present
    if f'"{name}"' in content:
        return False

    # Find the closing bracket of the list and insert before it
    list_match = re.search(
        rf'({list_name}\s*=\s*\[.*?)(\])',
        content,
        re.DOTALL,
    )
    if not list_match:
        return False

    new_entry = f'    "{name}",\n'
    content = content[: list_match.start(2)] + new_entry + content[list_match.start(2) :]

    _AGENT_PY.write_text(content)
    return True


@task(
    help={
        "name": "Snake_case name of the new check (e.g. 'my_check').",
        "path": "Go package directory to create the check in. Defaults to 'pkg/collector/corechecks'.",
        "windows_only": "Register in WINDOWS_CORECHECKS instead of AGENT_CORECHECKS.",
        "overwrite": "Overwrite existing files if they already exist.",
    }
)
def create(_, name, path="pkg/collector/corechecks", windows_only=False, overwrite=False):
    """
    Scaffold a new integration check.

    Creates:
      - cmd/agent/dist/assets/<name>/spec.yaml            (boilerplate spec)
      - cmd/agent/dist/conf.d/<name>.d/conf.yaml.example  (generated from spec)
      - <path>/<name>/<name>.go                            (Go check skeleton)
      - <path>/<name>/<name>_test.go                       (Go test skeleton)

    Also registers the check in pkg/commonchecks/corechecks.go and tasks/agent.py.
    Use --windows-only to add to WINDOWS_CORECHECKS instead of AGENT_CORECHECKS.
    """
    if not re.match(r'^[a-z][a-z0-9_]*$', name):
        raise Exit(
            f"Error: check name '{name}' must be lowercase alphanumeric with underscores.",
            code=1,
        )

    created = []
    modified = []

    # 1. Write boilerplate spec.yaml
    spec_dir = ASSETS_DIR / name
    spec_path = spec_dir / "spec.yaml"
    _maybe_write(spec_path, _boilerplate_spec_yaml(name), overwrite, created)

    # 2. Generate conf.yaml.example from the boilerplate spec
    spec = yaml.safe_load(spec_path.read_text())
    conf_dir = CONFD_DIR / f"{name}.d"
    conf_path = conf_dir / "conf.yaml.example"
    _maybe_write(conf_path, _generate_conf_yaml(name, spec), overwrite, created)

    # 3. Go check skeleton
    go_dir = Path(path) / name
    go_path = go_dir / f"{name}.go"
    test_path = go_dir / f"{name}_test.go"
    if windows_only:
        stub_path = go_dir / "stub.go"
        _maybe_write(stub_path, _render_stub_go(name), overwrite, created)
    _maybe_write(go_path, _render_check_go(name, windows_only), overwrite, created)
    _maybe_write(test_path, _render_check_test_go(name, windows_only), overwrite, created)

    # 4. Register in corechecks.go
    go_pkg_path = str(Path(path) / name)
    if _register_in_corechecks(name, go_pkg_path):
        modified.append(_CORECHECKS_GO)

    # 5. Add to AGENT_CORECHECKS or WINDOWS_CORECHECKS in tasks/agent.py
    if _register_in_agent_py(name, windows_only):
        modified.append(_AGENT_PY)

    print("\nCreated files:")
    for f in created:
        print(f"  {f}")

    if modified:
        list_name = "WINDOWS_CORECHECKS" if windows_only else "AGENT_CORECHECKS"
        print("\nModified files:")
        for f in modified:
            print(f"  {f}")
        print(f"  → Added '{name}' to {list_name} and registered in corechecks.go")

    print(
        f"\nNext steps:\n"
        f"  1. Edit {spec_path} to define your config parameters.\n"
        f"  2. Run `dda inv integration.spec-sync --check {name} --sync` to regenerate conf.yaml.example.\n"
        f"  3. Implement the check logic in {go_path}."
    )


def _maybe_write(path: Path, content: str, overwrite: bool, created: list):
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists() and not overwrite:
        raise Exit(
            f"Error: {path} already exists. Use --overwrite to replace it.",
            code=1,
        )
    path.write_text(content)
    created.append(path)


def _boilerplate_spec_yaml(name: str) -> str:
    spec = {
        "name": name,
        "fleet_configurable": False,
        "version": "1.0.0",
        "files": [
            {
                "name": f"{name}.yaml",
                "options": [
                    {
                        "template": "init_config",
                        "options": [{"template": "init_config/default"}],
                    },
                    {
                        "template": "instances",
                        "options": [{"template": "instances/default"}],
                    },
                ],
            }
        ],
    }
    return yaml.dump(spec, default_flow_style=False, sort_keys=False, allow_unicode=True)


def _check_name_to_type(name: str) -> str:
    """Convert snake_case to PascalCase."""
    return "".join(part.capitalize() for part in name.split("_"))


def _render_check_go(name: str, windows_only: bool = False) -> str:
    type_name = _check_name_to_type(name)
    copyright = _copyright_header()
    build_tag = "\n//go:build windows" if windows_only else ""
    return f"""{copyright}
{build_tag}

// Package {name} implements the {name} check.
package {name}

import (
\t"go.yaml.in/yaml/v2"

\t"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
\t"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
\t"github.com/DataDog/datadog-agent/pkg/collector/check"
\tcore "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
\t"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check.
const CheckName = "{name}"

// Configuration holds the instance-level configuration for {type_name}Check.
// Tags, service, and min_collection_interval are handled by CommonConfigure via CheckBase.
type Configuration struct {{
}}

// {type_name}Check implements the check.Check interface.
type {type_name}Check struct {{
\tcore.CheckBase
\tconfig Configuration
}}

// Factory returns a new instance of {type_name}Check.
func Factory() option.Option[func() check.Check] {{
\treturn option.New(newCheck)
}}

func newCheck() check.Check {{
\treturn &{type_name}Check{{
\t\tCheckBase: core.NewCheckBase(CheckName),
\t}}
}}

// Configure parses the check configuration.
func (c *{type_name}Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {{
\tif err := c.CommonConfigure(senderManager, initConfig, data, source); err != nil {{
\t\treturn err
\t}}
\treturn yaml.Unmarshal(data, &c.config)
}}

// Run executes the check.
func (c *{type_name}Check) Run() error {{
\tsender, err := c.GetSender()
\tif err != nil {{
\t\treturn err
\t}}

\t// TODO: implement check logic here
\t// sender.Gauge("metric.name", value, "", nil)

\tsender.Commit()
\treturn nil
}}
"""


def _render_stub_go(name: str) -> str:
    copyright = _copyright_header()
    return f"""{copyright}

//go:build !windows

package {name}

import (
\t"github.com/DataDog/datadog-agent/pkg/collector/check"
\t"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check.
const CheckName = "{name}"

// Factory returns None on non-Windows platforms.
func Factory() option.Option[func() check.Check] {{
\treturn option.None[func() check.Check]()
}}
"""


def _render_check_test_go(name: str, windows_only: bool = False) -> str:
    type_name = _check_name_to_type(name)
    copyright = _copyright_header()
    build_tag = "\n//go:build windows" if windows_only else ""
    return f"""{copyright}
{build_tag}

package {name}

import (
\t"testing"

\t"github.com/stretchr/testify/assert"
)

func TestCheckName(t *testing.T) {{
\tcheck := newCheck().(*{type_name}Check)
\tassert.Equal(t, CheckName, string(check.ID()))
}}
"""


def _copyright_header() -> str:
    import datetime

    year = datetime.datetime.now().year
    return (
        f"// Unless explicitly stated otherwise all files in this repository are licensed\n"
        f"// under the Apache License Version 2.0.\n"
        f"// This product includes software developed at Datadog (https://www.datadoghq.com/).\n"
        f"// Copyright {year}-present Datadog, Inc."
    )
