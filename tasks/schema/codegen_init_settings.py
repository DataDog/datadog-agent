import ast
import os
import re

from tasks.libs.types.version import Version

file_header_template = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// NOTE! This is a generated file, do not modify it. Created by `dda inv schema.codegen`

package {package}
"""

file_header = file_header_template.format(package="setup")
constants_file_header = file_header_template.format(package="constants")

constant_header = """//
// The following code is generated from the schema and should never be manually edited
//
"""


def render_settings_file(funcs):
    """Render a settings file: header, the imports its body needs, then each function.

    funcs - list of (Go function declaration line, body lines)
    """
    body = []
    for declaration, lines in funcs:
        if body:
            body.append('')
        body += [declaration, *lines, '}']

    imports = ['import (']
    if any('time.' in line for line in body):
        imports += ['\t"time"', '']
    if any('pkgconfighelper.' in line for line in body):
        imports += ['\tpkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"']
    imports += ['\tpkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"', ')', '']

    return file_header.split('\n') + imports + body


def walk_settings(schema, prefix='', full_agent_only=False):
    """Yield (dotted path, node, full_agent_only) for every setting in the schema.

    full_agent_only is True when the setting, or one of its parent sections, is
    tagged `full-agent-only:true`.
    """
    for field, node in schema['properties'].items():
        path = f"{prefix}.{field}" if prefix else field
        tagged = full_agent_only or 'full-agent-only:true' in (node.get('tags') or [])
        node_type = node.get('node_type', 'setting')
        if node_type == 'setting':
            yield path, node, tagged
        elif node_type == 'section':
            yield from walk_settings(node, path, tagged)


def walk_sections(schema, prefix=''):
    """Yield (dotted path, node) for every section in the schema."""
    for field, node in schema['properties'].items():
        if node.get('node_type') != 'section':
            continue
        path = f"{prefix}.{field}" if prefix else field
        yield path, node
        yield from walk_sections(node, path)


def try_parse_duration(text):
    if not isinstance(text, str):
        return None
    m = re.fullmatch(r'(-)?(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?(?:(\d+)ms)?(?:(\d+)µs)?(?:(\d+)ns)?', text)
    if not m or not any(m.groups()):
        return None
    minus = m.group(1) or False
    hours = int(m.group(2) or 0)
    minutes = int(m.group(3) or 0)
    seconds = int(m.group(4) or 0)
    millis = int(m.group(5) or 0)
    micros = int(m.group(6) or 0)
    nanos = int(m.group(7) or 0)
    parts = []
    if hours:
        parts.append('%d*time.Hour' % hours)
    if minutes:
        parts.append('%d*time.Minute' % minutes)
    if seconds:
        parts.append('%d*time.Second' % seconds)
    if millis:
        parts.append('%d*time.Millisecond' % millis)
    if micros:
        parts.append('%d*time.Microsecond' % micros)
    if nanos:
        parts.append('%d*time.Nanosecond' % nanos)
    if not parts:
        return 'time.Duration(0)'
    if minus and len(parts) == 1:
        return f"-{parts[0]}"
    elif minus:
        return f"-({' + '.join(parts)})"
    return ' + '.join(parts)


def value_to_gostr(obj):
    if isinstance(obj, str) and '\\.' in obj:
        # regex-like strings need to use backtick (`) quotes
        return f"`{obj}`"
    if isinstance(obj, str):
        return f"\"{obj}\""
    if isinstance(obj, bool):
        return 'true' if obj else 'false'
    return str(obj)


def as_go_value(text, split_lines=False):
    if not isinstance(text, str):
        text = str(text)

    obj = ast.literal_eval(text)
    res = []

    if isinstance(obj, list):
        if len(text) >= 60 or len(obj) > 6:
            split_lines = True
        for elem in obj:
            res.append(value_to_gostr(elem))
    else:  # assume dict/map
        width = max((len(value_to_gostr(k)) for k in obj), default=0)
        for k, v in obj.items():
            key = value_to_gostr(k) + ':'
            res.append(f"{key.ljust(width + 1)} {value_to_gostr(v)}")

    if split_lines:
        return f"{{\n\t\t{',\n\t\t'.join(res)},\n\t}}"
    return f"{{{', '.join(res)}}}"


def get_golang_type_tag(curr):
    tags = curr.get('tags')
    if not tags:
        return None
    for t in tags:
        if ':' in t:
            (k, v) = t.split(':', 1)
            if k == 'golang_type':
                return v
    return None


def retrieve_default_value(node, name):
    settingDefault = node.get('default')
    settingType = node.get('type')

    if node.get('platform_default'):
        platform_default = as_go_value(node['platform_default'], split_lines=True)
        return f"getPlatformDefault(map[string]interface{{}}{platform_default})"

    if settingType in ('array', 'object'):
        return to_vartype(node, as_go_value(settingDefault))

    if settingType == 'boolean':
        return 'true' if settingDefault else 'false'

    if settingType == 'integer':
        if get_golang_type_tag(node) == 'int64':
            return f"int64({settingDefault})"
        return str(settingDefault)

    if settingType == 'number':
        if isinstance(settingDefault, float):
            textDefault = str(settingDefault)
            if '.' in textDefault:
                return str(settingDefault)
            return f"float64({settingDefault}.0)"
        if isinstance(settingDefault, int):
            return f"{settingDefault}.0"

    if settingType == 'string':
        if node.get('format') == 'duration':
            # time.Duration are specially rendered
            durationValue = try_parse_duration(settingDefault)
            if durationValue is not None:
                return durationValue
        if isinstance(settingDefault, str):
            return f"\"{settingDefault}\""

    raise RuntimeError(
        f"setting {name}: cant handle settingType: '{settingType}', settingDefault: '{settingDefault}' of {type(settingDefault)}"
    )


def dict_to_gotype(inp):
    """Convert a node of json schema into a golang type expression string"""
    if inp is None:
        return 'interface{}'
    elif inp.get('type') == 'integer':
        return 'int'
    elif inp.get('type') == 'number':
        return 'float64'
    elif inp.get('type') == 'string':
        return 'string'
    elif inp.get('type') == 'array':
        return f"[]{dict_to_gotype(inp.get('items'))}"
    elif inp.get('type') == 'object':
        return f"map[string]{dict_to_gotype(inp.get('additionalProperties'))}"


def to_vartype(node, setting_default):
    if node.get('type') == 'array':
        tags = node.get('tags')
        if tags:
            if 'golang_type:[]int' in tags:
                return f"[]int{setting_default}"
    return f"{dict_to_gotype(node)}{setting_default}"


# env_parser value -> (Go function, call shape). Call shapes:
#   'config'  -> config.<func>(key)
#   'vartype' -> config.<func>(key, <zero value of the setting's Go type>)
#   'helper'  -> pkgconfighelper.<func>(key, config)
ENV_PARSERS = {
    'comma_separated': ('ParseEnvSplitComma', 'config'),
    'space_separated': ('ParseEnvSplitSpace', 'config'),
    'json': ('ParseEnvJSON', 'vartype'),
    'comma_and_space_separated': ('ParseEnvSplitCommaAndSpace', 'helper'),
    'traces_span': ('ParseEnvTraceSpan', 'helper'),
    'csv_comma_separated': ('ParseEnvCSVSplit', 'helper'),
    'comma_then_space_separated': ('ParseEnvSplitCommaThenSpace', 'helper'),
    'json_list_or_comma_separated': ('ParseEnvJSONOrComma', 'helper'),
    'json_list_or_space_separated': ('ParseEnvJSONOrSpace', 'helper'),
}


def env_parser_to_func_call(name, env_parser, node):
    parser_func, shape = ENV_PARSERS[env_parser]
    if shape == 'helper':
        return f"\tpkgconfighelper.{parser_func}(\"{name}\", config)"
    if shape == 'vartype':
        return f"\tconfig.{parser_func}(\"{name}\", {to_vartype(node, '{}')})"
    return f"\tconfig.{parser_func}(\"{name}\")"


def deprecated_names(node):
    """Return the former names of a setting, oldest deprecation first.

    'renamed_from' maps each former name to the Agent version that deprecated it. The config gives
    former names priority over the canonical one, oldest first, so the emitted order matters.
    """
    renamed_from = node.get('renamed_from') or {}
    return sorted(renamed_from, key=lambda name: (Version.from_tag(renamed_from[name]), name))


def setting_sourcecode(name, node):
    """Return the Go lines declaring a single setting."""
    settingname = '"%s"' % name
    defaultval = retrieve_default_value(node, name)
    envsuffix = ''.join(', "%s"' % ev for ev in node.get('env_vars') or [])

    # method used to declare the setting
    tags = node.get('tags', [])
    renamed_from = deprecated_names(node)
    if 'no-env' in tags:
        line = f"\tconfig.SetDefault({settingname}, {defaultval})"
    elif renamed_from:
        deprecated = ', '.join('"%s"' % n for n in renamed_from)
        line = f"\tconfig.BindEnvAndSetDefaultWithDeprecation({settingname}, {defaultval}, []string{{{deprecated}}}{envsuffix})"
    else:
        line = f"\tconfig.BindEnvAndSetDefault({settingname}, {defaultval}{envsuffix})"

    sourcecode = [line]

    # only after the setting is defined should the env parser appear
    env_parser = node.get('env_parser')
    if env_parser:
        sourcecode.append(env_parser_to_func_call(name, env_parser, node))

    return sourcecode


def gen_delegated_auth_map(core_schema, system_probe_schema):
    """
    Generate the delegated auth constant map for pkg/config/setup/generated.go.

    core_schema          - loaded core schema object
    system_probe_schema  - loaded system-probe schema object, unused
    """
    out = file_header.split('\n') + constant_header.split('\n')
    out.append("""type delegatedAuthConfig struct {
	apiKeyPath        string
	delegatedAuthPath string
	description       string
}

// delegatedAuthKeys list all the \"delegated_auth\" configuration section.
// This list is used to fully initialize authentication through cloud provider instead of API key
var delegatedAuthKeys = []delegatedAuthConfig{""")

    for path, _ in walk_sections(core_schema):
        if path.split('.')[-1] != 'delegated_auth':
            continue
        # the section holding this delegated_auth, '' for the root one
        parent = path.rsplit('.', 1)[0] if '.' in path else ''
        prefix = f"{parent}." if parent else ""
        out.append(f"""	{{
		apiKeyPath:        "{prefix}api_key",
		delegatedAuthPath: "{prefix}delegated_auth",
		description:       "{parent or 'global'}",
	}},""")
    out.append("}")
    return out


def run_core_constant_codegen(core_schema, system_probe_schema, outsource_dir):
    """
    Generate pkg/config/setup/generated.go: the delegated auth constant map.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    outsource_dir       - the directory to output source code to
    """
    print('Output generated.go')
    os.makedirs(outsource_dir, exist_ok=True)
    _write_uniform_lines(
        os.path.join(outsource_dir, "generated.go"), gen_delegated_auth_map(core_schema, system_probe_schema)
    )


GENERATE_CONST_PREFIX = "generate_const:"


def gen_generate_const(core_schema, system_probe_schema):
    """
    Generate the `const` block for pkg/config/setup/constants/generated.go, declaring every Go
    constant referenced by a `generate_const:<name>` tag, set to its associated setting's
    default value. Returns None if no setting carries that tag.

    Both schemas are traversed. A constant may be referenced by several settings (in either schema);
    they must all resolve to the same default value, otherwise codegen fails — a single constant
    cannot have an ambiguous value.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    """
    # const name -> {'value': go_value, 'source': setting_keypath}
    consts = {}

    def collect(schema):
        for keyname, node, _ in walk_settings(schema):
            for tag in node.get('tags') or []:
                if not isinstance(tag, str) or not tag.startswith(GENERATE_CONST_PREFIX):
                    continue
                name = tag[len(GENERATE_CONST_PREFIX) :]
                value = retrieve_default_value(node, keyname)
                existing = consts.get(name)
                if existing is not None and existing['value'] != value:
                    raise RuntimeError(
                        f"generate_const '{name}' has conflicting default values: "
                        f"'{existing['source']}' => {existing['value']} vs '{keyname}' => {value}. "
                        f"A generated constant must have a single value; tag only the setting whose "
                        f"default is exactly the constant, or make the defaults agree."
                    )
                if existing is None:
                    consts[name] = {'value': value, 'source': keyname}

    collect(core_schema)
    collect(system_probe_schema)

    if not consts:
        return None

    out = constants_file_header.split('\n') + constant_header.split('\n')
    out.append("// Constants generated from settings tagged with a `generate_const:<name>` label.")
    out.append("// Each constant's value is the default of its associated setting.")
    out.append("const (")
    width = max(len(name) for name in consts)
    for name in sorted(consts):
        out.append(f"\t{name.ljust(width)} = {consts[name]['value']}")
    out.append(")")
    return out


def run_constants_codegen(core_schema, system_probe_schema, outsource_dir):
    """
    Generate pkg/config/setup/constants/generated.go.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    outsource_dir       - the directory to output source code to
    """
    lines = gen_generate_const(core_schema, system_probe_schema)
    if lines is None:
        return

    print('Output constants/generated.go')
    os.makedirs(outsource_dir, exist_ok=True)
    _write_uniform_lines(os.path.join(outsource_dir, "generated.go"), lines)


def _write_uniform_lines(path, lines):
    """
    Write lines to path, one per line, so the bytes never depend on the host platform.

    Text mode would otherwise translate newlines to os.linesep, yielding CRLF on Windows.
    """
    with open(path, "w", newline="\n") as f:
        for line in lines:
            print(line, file=f)


def run_codegen(schema, outsource_dir, sysprobe=False):
    """
    Entry point for code generation.
    schema        - loaded schema object (dict with schema['properties'])
    outsource_dir - the directory to output source code to
    sysprobe      - generate the system-probe file instead of the core-agent one
    """
    output_full_agent = []
    output_common_base = []
    for path, node, full_agent_only in walk_settings(schema):
        if full_agent_only:
            output_full_agent += setting_sourcecode(path, node)
        else:
            output_common_base += setting_sourcecode(path, node)

    if sysprobe:
        filename = 'system_probe_settings.go'
        sourcecode = render_settings_file(
            [('func initMainSystemProbeConfig(config pkgconfigmodel.Setup) {', output_common_base)]
        )
    else:
        filename = 'all_settings.go'
        sourcecode = render_settings_file(
            [
                ('func initCoreAgentFull(config pkgconfigmodel.Setup) {', output_full_agent),
                ('func initCommonBase(config pkgconfigmodel.Setup) {', output_common_base),
            ]
        )

    print('Output %s' % filename)
    _write_uniform_lines(os.path.join(outsource_dir, filename), sourcecode)
