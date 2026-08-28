import ast
import os
import re

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


class BufferedSetting:
    def __init__(self, path, sourcecode):
        self.path = path
        self.sourcecode = sourcecode
        self.done = False


class CodeGeneratorTarget:
    def __init__(self):
        self.buffer = None
        self.output_full_agent = []
        self.output_common_base = []
        self.header_text = None
        self.filesystem = None

    def add_header(self, text):
        self.header_text = text

    def add(self, path, schema, sourcecode):
        if self.buffer is None:
            if retrieve_output_mode(path.split('.'), schema) == 'full-agent-only':
                self.output_full_agent += sourcecode
            else:
                self.output_common_base += sourcecode
            return
        self.buffer[path] = BufferedSetting(path, sourcecode)

    def _add_imports(self, need_pkgconfighelper, need_time):
        sourcecode = ['import (']
        if need_time:
            sourcecode += ['\t"time"']
            sourcecode += ['']
        if need_pkgconfighelper:
            sourcecode += ['\tpkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"']
        sourcecode += ['\tpkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"']
        sourcecode += [')', '']
        return sourcecode

    def output_result_for_all_settings(self, filename_filter):
        if filename_filter("system_probe_settings.go"):
            return self.output_result_for_sysprobe_settings()
        return self.output_result_for_core_agent_settings()

    def output_result_for_sysprobe_settings(self):
        res = self.header_text.split('\n')
        res += self._add_imports(False, contains_import(self.output_common_base, 'time'))
        res += ['func initMainSystemProbeConfig(config pkgconfigmodel.Setup) {']
        res += self.output_common_base
        res += ['}']
        self.filesystem = {'system_probe_settings.go': res}

    def output_result_for_core_agent_settings(self):
        res = self.header_text.split('\n')
        res += self._add_imports(True, contains_import(self.output_full_agent, 'time'))
        res += ['func initCoreAgentFull(config pkgconfigmodel.Setup) {']
        res += self.output_full_agent
        res += ['}', '']
        res += ['func initCommonBase(config pkgconfigmodel.Setup) {']
        res += self.output_common_base
        res += ['}']
        self.filesystem = {'all_settings.go': res}

    def write_to_directory(self, out_dir, filename_filter):
        for filename in self.filesystem:
            if filename_filter and not filename_filter(filename):
                print('Skipping %s' % filename)
                continue
            print('Output %s' % filename)
            out_filename = os.path.join(out_dir, filename)
            _write_uniform_lines(out_filename, self.filesystem[filename])


def _write_uniform_lines(path, lines):
    """
    Write lines to path, one per line, so the bytes never depend on the host platform.

    Text mode would otherwise translate newlines to os.linesep, yielding CRLF on Windows.
    """
    with open(path, "w", newline="\n") as f:
        for line in lines:
            print(line, file=f)


def join_key(prefix, field):
    if prefix == '':
        return field
    if prefix.endswith('.'):
        return f"{prefix}{field}"
    return f"{prefix}.{field}"


def contains_import(sourcecode, symbol):
    if not isinstance(sourcecode, list) and not isinstance(sourcecode[0], str):
        raise RuntimeError('sourcecode must be a list of strings')
    needle = f"{symbol}."
    for line in sourcecode:
        if needle in line:
            return True
    return False


def _is_node_leaf(node):
    if 'node_type' not in node:
        return True
    return node['node_type'] == 'setting'


def _is_node_section(node):
    if 'node_type' not in node:
        return False
    return node['node_type'] == 'section'


def walk_schema(schema, curr_path, callback):
    child_nodes = schema['properties']
    for field in child_nodes:
        next_path = join_key(curr_path, field)
        node = child_nodes[field]
        if _is_node_leaf(node):
            callback(next_path)
        elif _is_node_section(node):
            walk_schema(node, next_path, callback)


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
    if isinstance(obj, bool) and obj:
        return 'true'
    if isinstance(obj, bool) and not obj:
        return 'false'
    if isinstance(obj, int):
        return str(obj)
    return obj


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
        indent_size = calc_indent_size(obj)
        for k, v in obj.items():
            key = value_to_gostr(k)
            val = value_to_gostr(v)
            pad_space = calc_pad_space(key, indent_size)
            res.append(f"{key}:{' ' * pad_space} {val}")

    if split_lines:
        return f"{{\n\t\t{',\n\t\t'.join(res)},\n\t}}"
    return f"{{{', '.join(res)}}}"


def calc_indent_size(obj):
    max_size = 0
    for k in obj.keys():
        key = value_to_gostr(k)
        if len(key) > max_size:
            max_size = len(key)
    return max_size


def calc_pad_space(lhs, indent_size):
    return max(indent_size - len(lhs), 0)


def get_golang_type_tag(curr):
    tags = curr.get('tags')
    if not tags:
        return None
    for t in tags:
        if ':' in t:
            (k, v) = t.split(':')
            if k == 'golang_type':
                return v
    return None


def get_node(keypath, schema):
    curr = schema
    for k in keypath:
        curr = curr['properties']
        curr = curr[k]
    return curr


def retrieve_output_mode(keypath, schema):
    for i in range(0, len(keypath)):
        # Iterate the keypath bottom-up, for example 'a.b.c' -> ['a.b.c', 'a.b', 'a']
        subpath = keypath[0 : len(keypath) - i]
        node = get_node(subpath, schema)
        tags = node.get('tags')
        if tags and 'full-agent-only:true' in tags:
            return 'full-agent-only'
    return None


def retrieve_default_value(keypath, schema):
    node = get_node(keypath, schema)
    settingDefault = node.get('default')
    settingType = node.get('type')
    if settingType is None:
        return 'nil'

    if node.get('platform_default'):
        platform_default = as_go_value(node['platform_default'], split_lines=True)
        return f"getPlatformDefault(map[string]interface{{}}{platform_default})"

    if settingType == 'array' or settingType == 'object':
        return to_vartype(node, as_go_value(settingDefault))

    elif settingType == 'boolean':
        if settingDefault:
            return 'true'
        return 'false'

    elif settingType == 'integer':
        if get_golang_type_tag(node) == 'int64':
            return f"int64({settingDefault})"
        if get_golang_type_tag(node) == 'float64':
            return f"float64({settingDefault})"
        durationValue = try_parse_duration(settingDefault)
        if durationValue is not None:
            return str(durationValue)
        if settingDefault is None:
            return '0'
        return str(settingDefault)

    elif settingType == 'number':
        if get_golang_type_tag(node) == 'int64':
            return f"int64({settingDefault})"
        if get_golang_type_tag(node) == 'float64':
            return f"float64({settingDefault})"
        durationValue = try_parse_duration(settingDefault)
        if durationValue is not None:
            return str(durationValue)
        if settingDefault is None:
            return '0'
        if isinstance(settingDefault, float):
            textDefault = str(settingDefault)
            if '.' in textDefault:
                return str(settingDefault)
            return f"float64({settingDefault}.0)"
        if isinstance(settingDefault, int):
            return str(settingDefault)

    elif settingType == 'string':
        if node.get('format') == 'duration':
            # time.Duration are specially rendered
            durationValue = try_parse_duration(settingDefault)
            if durationValue is not None:
                return str(durationValue)
        if settingDefault is None:
            return '""'
        if isinstance(settingDefault, str):
            return f"\"{settingDefault}\""

    elif settingType == 'object':
        textDefault = str(settingDefault)
        add = node.get('additionalProperties')
        if add is not None:
            if add.get('type') == 'string':
                return f"map[string]string{as_go_value(settingDefault)}"
            if add.get('type') == 'array' and add.get('items').get('type') == 'string':
                return f"map[string][]string{as_go_value(settingDefault)}"
        return f"map[string]interface{{}}{as_go_value(settingDefault)}"
    raise RuntimeError(
        f"setting {keypath}: cant handle settingType: '{settingType}', settingDefault: '{settingDefault}' of {type(settingDefault)}"
    )


def retrieve_envvars(keypath, schema):
    node = get_node(keypath, schema)
    envvars = node.get('env_vars')
    return envvars


def retrieve_env_parser(keypath, schema):
    node = get_node(keypath, schema)
    return node.get('env_parser')


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
    # No 'type', for instance a 'oneOf' of several types: the setting holds
    # whatever the config layer decoded.
    return 'interface{}'


def to_vartype(node, setting_default):
    if node.get('type') == 'array':
        tags = node.get('tags')
        if tags:
            if 'golang_type:[]int' in tags:
                return f"[]int{setting_default}"
    return f"{dict_to_gotype(node)}{setting_default}"


def retrieve_method_to_declare(keypath, schema):
    node = get_node(keypath, schema)
    tags = node.get('tags')
    if tags:
        if 'no-env' in tags:
            return 'SetDefault'
    return 'BindEnvAndSetDefault'


def env_parser_to_func_call(name, env_parser, get_vartype):
    parser_func = None
    is_method_key_vartype = False
    is_helper_key_config = False

    if env_parser == 'comma_separated':
        parser_func = 'ParseEnvSplitComma'
    elif env_parser == 'space_separated':
        parser_func = 'ParseEnvSplitSpace'
    elif env_parser == 'json':
        parser_func = 'ParseEnvJSON'
        is_method_key_vartype = True
    elif env_parser == 'comma_and_space_separated':
        parser_func = 'ParseEnvSplitCommaAndSpace'
        is_helper_key_config = True
    elif env_parser == 'traces_span':
        parser_func = 'ParseEnvTraceSpan'
        is_helper_key_config = True
    elif env_parser == 'csv_comma_separated':
        parser_func = 'ParseEnvCSVSplit'
        is_helper_key_config = True
    elif env_parser == 'comma_then_space_separated':
        parser_func = 'ParseEnvSplitCommaThenSpace'
        is_helper_key_config = True
    elif env_parser == 'json_list_or_comma_separated':
        parser_func = 'ParseEnvJSONOrComma'
        is_helper_key_config = True
    elif env_parser == 'json_list_or_space_separated':
        parser_func = 'ParseEnvJSONOrSpace'
        is_helper_key_config = True

    if is_helper_key_config:
        return f"\tpkgconfighelper.{parser_func}(\"{name}\", config)"
    if is_method_key_vartype:
        var_type = get_vartype()
        return f"\tconfig.{parser_func}(\"{name}\", {var_type})"
    return f"\tconfig.{parser_func}(\"{name}\")"


# Create source code for a single setting, add to the target
def output_single_setting(name, internal_comment, schema, target):
    sourcecode = []

    # basic info: name, default value, env vars
    settingname = '"%s"' % name
    defaultval = retrieve_default_value(name.split('.'), schema)
    envsuffix = ''
    envvars = retrieve_envvars(name.split('.'), schema)
    if envvars is not None and len(envvars) > 0:
        envvars = ['"%s"' % ev for ev in envvars]
        envsuffix = ', ' + ', '.join(envvars)

    # get env parser function, don't output yet
    env_parser = retrieve_env_parser(name.split('.'), schema)

    # internal-only comments for the setting
    if internal_comment:
        for text in internal_comment.split('\n'):
            sourcecode.append('\t// %s' % text)

    # method name to use for declaring the setting
    method_name = retrieve_method_to_declare(name.split('.'), schema)
    if method_name == 'BindEnvAndSetDefault':
        line = f"\tconfig.BindEnvAndSetDefault({settingname}, {defaultval}{envsuffix})"
    elif method_name == 'SetDefault':
        line = f"\tconfig.SetDefault({settingname}, {defaultval})"
    else:
        raise RuntimeError(f"unknown method name {method_name} for setting {name}")

    # the line of code that defines the setting
    sourcecode.append(line)

    # only after the setting is defined should the env parser appear
    if env_parser:

        def get_vartype():
            node = get_node(name.split('.'), schema)
            return to_vartype(node, '{}')

        line = env_parser_to_func_call(name, env_parser, get_vartype)
        sourcecode.append(line)

    # write to our target
    target.add(name, schema, sourcecode)


def gen_delegated_auth_map(core_schema, system_probe_schema, outputs):
    """
    Constant generator: appends the delegated auth map to the relevant buffers.

    core_schema           - loaded core schema object
    system_probe_schema  - loaded system-probe schema object, unused
    outputs               - map of output name (see `constant_outputs`) to its Go source lines
    """

    def collect_delegated_auth_keys(schema):
        keys = []

        # Visitor for each setting
        def visit(curr_path, node):
            if node.get("node_type") == "setting":
                return

            for name, child in node["properties"].items():
                if name == "delegated_auth":
                    keys.append(curr_path)
                else:
                    path = curr_path + "." + name if curr_path else name
                    visit(path, child)

        visit("", schema)
        return keys

    def emit(out, keys):
        out.append("""type delegatedAuthConfig struct {
	apiKeyPath        string
	delegatedAuthPath string
	description       string
}

// delegatedAuthKeys list all the \"delegated_auth\" configuration section.
// This list is used to fully initialize authentication through cloud provider instead of API key
var delegatedAuthKeys = []delegatedAuthConfig{""")

        for key in keys:
            parent_section_name = key.rsplit(".")[0]
            parent_section = key.rsplit(".")[0]

            if parent_section != "":
                parent_section += "."
            if parent_section_name == "":
                parent_section_name = "global"

            out.append(f"""	{{
		apiKeyPath:        "{parent_section}api_key",
		delegatedAuthPath: "{parent_section}delegated_auth",
		description:       "{parent_section_name}",
	}},""")
        out.append("}")

    emit(outputs["core"], collect_delegated_auth_keys(core_schema))


GENERATE_CONST_PREFIX = "generate_const:"


def gen_generate_const(core_schema, system_probe_schema, outputs):
    """
    Constant generator: emits a `const` block declaring every Go constant referenced by a
    `generate_const:<name>` tag, set to its associated setting's default value.

    The block goes to the `constants` output, ie. the `pkg/config/setup/constants` package, so
    that code can use those constants without importing the whole `setup` package.

    Both schemas are traversed. A constant may be referenced by several settings (in either schema);
    they must all resolve to the same default value, otherwise codegen fails — a single constant
    cannot have an ambiguous value.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    outputs             - map of output name (see `constant_outputs`) to its Go source lines
    """
    # const name -> {'value': go_value, 'source': setting_keypath}
    consts = {}

    def collect(schema):
        def visit(keyname):
            node = get_node(keyname.split('.'), schema)
            for tag in node.get('tags') or []:
                if not isinstance(tag, str) or not tag.startswith(GENERATE_CONST_PREFIX):
                    continue
                name = tag[len(GENERATE_CONST_PREFIX) :]
                value = retrieve_default_value(keyname.split('.'), schema)
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

        walk_schema(schema, '', visit)

    collect(core_schema)
    collect(system_probe_schema)

    if not consts:
        return

    out = outputs["constants"]
    out.append("// Constants generated from settings tagged with a `generate_const:<name>` label.")
    out.append("// Each constant's value is the default of its associated setting.")
    out.append("const (")
    magic_value = calc_const_indent(consts)
    for name in sorted(consts):
        pad_space = magic_value - len(name)
        out.append(f"\t{name}{' ' * pad_space} = {consts[name]['value']}")
    out.append(")")


# The files produced by the constant generators, keyed by the output name generators use to
# reach them. Each entry is (path relative to the codegen output dir, Go file header). The
# relative path mirrors the layout under `pkg/config/setup`, so `schema.codegen` knows where
# each file has to be copied.
constant_outputs = {
    "core": ("generated.go", file_header),
    "system_probe": ("system_probe_generated.go", file_header),
    "constants": (os.path.join("constants", "generated.go"), constants_file_header),
}


# Ordered list of generator functions used to produce the constant files.
# Each is called with (core_schema, system_probe_schema, outputs) and may append Go code to
# any of the `constant_outputs` buffers.
constant_generators = [
    gen_delegated_auth_map,
    gen_generate_const,
]


def calc_const_indent(list_names):
    max_size = 0
    for name in list_names:
        if len(name) > max_size:
            max_size = len(name)
    return max_size


def run_constant_codegen(core_schema, system_probe_schema, outsource_dir):
    """
    Generate the constant files by running each generator in `constant_generators` in order.
    Each generator receives both schemas and every output buffer, so it can append Go code to
    any of the `constant_outputs` files.

    Outputs no generator wrote anything to are skipped rather than emitted as header-only
    files.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    outsource_dir       - the directory to output source code to
    """
    header_lines = {
        name: header.split('\n') + constant_header.split('\n') for name, (_, header) in constant_outputs.items()
    }
    outputs = {name: list(lines) for name, lines in header_lines.items()}

    for generator in constant_generators:
        generator(core_schema, system_probe_schema, outputs)

    for name, (filename, _) in constant_outputs.items():
        sourcecode = outputs[name]
        if sourcecode == header_lines[name]:
            continue

        print('Output %s' % filename)
        out_filename = os.path.join(outsource_dir, filename)
        os.makedirs(os.path.dirname(out_filename), exist_ok=True)
        _write_uniform_lines(out_filename, sourcecode)


def run_codegen(schema, filename_filter, outsource_dir):
    """
    Entry point for code generation.
    schema          - loaded schema object (dict with schema['properities'])
    filename_filter - optional function to filter output filenames (or None)
    outsource_dir   - the directory to output source code to
    """
    target = CodeGeneratorTarget()
    target.add_header(file_header)

    # Visitor for each setting
    def process_single_setting(keyname):
        internal_comment = []
        output_single_setting(keyname, internal_comment, schema, target)

    # walk the schema to generate code
    walk_schema(schema, '', process_single_setting)
    target.output_result_for_all_settings(filename_filter)

    target.write_to_directory(outsource_dir, filename_filter)
