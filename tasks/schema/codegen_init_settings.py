import os
import re
import subprocess

file_header = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// NOTE! This is a generated file, do not modify it. Created by `dda inv schema.codegen`

package setup
"""

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
        self.output_everything = []
        self.header_text = None
        self.filesystem = None

    def use_func_order(self, hints, func_names):
        self.reorder_hints = hints
        self.reorder_func_names = func_names
        self.buffer = {}

    def add_header(self, text):
        self.header_text = text

    def add(self, path, schema, sourcecode):
        if self.buffer is None:
            if retrieve_output_mode(path.split('.'), schema) == 'full-agent-only':
                self.output_full_agent += sourcecode
            else:
                self.output_everything += sourcecode
            return
        self.buffer[path] = BufferedSetting(path, sourcecode)

    def flush_buffer(self, filename_filter):
        # Without the --keep-orig-order flag, output everything at once
        if self.buffer is None:
            self.output_result_for_all_settings(filename_filter)
            return

        # Otherwise, we output multiple source files
        self.filesystem = {}
        sourcecode = None

        # Run over the re-ordering function list, retrieve from buffered codegen
        for funcname in self.reorder_func_names:
            h = retrieve_func_order(self.reorder_hints, funcname)
            if not h:
                print(f"[WARN] not found: {funcname}")
                continue
            (filename, settings) = h['filename'], h['settings']
            if filename_filter and not filename_filter(filename):
                continue

            # Get filename to write to, add header if its empty
            if filename not in self.filesystem:
                self._add_file_header(filename, settings)

            # Create the function, declare all settings in it
            sourcecode = self.filesystem[filename]
            output_func_header(funcname, sourcecode)
            for row in settings:
                # pattern
                if row[1].startswith('pattern_'):
                    suffix_list = get_suffixes_for_pattern(row[1])
                    for suffix in suffix_list:
                        keyname = join_key(row[0], suffix)
                        if keyname not in self.buffer:
                            continue
                        setting = self.buffer[keyname]
                        self.buffer[keyname].done = True
                        sourcecode = sourcecode + setting.sourcecode
                    continue
                # single setting
                keyname = row[0]
                if keyname not in self.buffer:
                    continue
                setting = self.buffer[keyname]
                self.buffer[keyname].done = True
                sourcecode = sourcecode + setting.sourcecode
            output_func_footer(funcname, sourcecode)
            self.filesystem[filename] = sourcecode

        # Afterwards: run over buffer to get everything else
        other_filename = 'other_settings.go'
        self._add_file_header(other_filename, [])
        sourcecode = self.filesystem[other_filename]
        output_func_header("otherSettings", sourcecode)
        for keyname in self.buffer:
            if self.buffer[keyname].done:
                continue
            self.buffer[keyname].done = True
            sourcecode = sourcecode + self.buffer[keyname].sourcecode
        output_func_footer("otherSettings", sourcecode)
        self.filesystem[other_filename] = sourcecode

    def _add_file_header(self, filename, settings):
        self.filesystem[filename] = self.header_text.split('\n')
        # Determine if the target file needs to import pkgconfighelper
        need_pkgconfighelper = False
        for row in settings:
            keyname = row[0]
            setting = self.buffer.get(keyname)
            if setting:
                for line in setting.sourcecode:
                    if 'pkgconfighelper.' in line:
                        need_pkgconfighelper = True
        self.filesystem[filename] += self._add_imports(need_pkgconfighelper)

    def _add_imports(self, need_pkgconfighelper):
        sourcecode = ['import (']
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
        res += self._add_imports(False)
        res += ['func initSystemProbeConfig(config pkgconfigmodel.Setup) {']
        res += self.output_everything
        res += ['}']
        self.filesystem = {'system_probe_settings.go': res}

    def output_result_for_core_agent_settings(self):
        res = self.header_text.split('\n')
        res += self._add_imports(False)
        res += ['func initCoreAgentFull(config pkgconfigmodel.Setup) {']
        res += self.output_full_agent
        res += ['}', '']
        res += ['func initEverything(config pkgconfigmodel.Setup) {']
        res += self.output_everything
        res += ['}']
        self.filesystem = {'all_settings.go': res}

    def write_to_directory(self, out_dir, filename_filter):
        for filename in self.filesystem:
            if filename_filter and not filename_filter(filename):
                print('Skipping %s' % filename)
                continue
            print('Output %s' % filename)
            out_filename = os.path.join(out_dir, filename)
            with open(out_filename, "w") as f:
                f.write('\n'.join(self.filesystem[filename]))


def join_key(prefix, field):
    if prefix == '':
        return field
    if prefix.endswith('.'):
        return f"{prefix}{field}"
    return f"{prefix}.{field}"


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


def retrieve_hint(hints_obj, keyname):
    if hints_obj is None:
        return None
    for perFilenameFuncSettings in hints_obj:
        for row in perFilenameFuncSettings['settings']:
            if row[0] == keyname:
                return {'kind': row[1], 'internal_comment': row[2]}
            elif row[1].startswith('pattern_') and keyname.startswith(row[0]):
                # When multiple settings are created for a prefix, only add the
                # comment to the first such setting.
                internal_comment = row[2]
                row[2] = ''
                return {'kind': row[1], 'internal_comment': internal_comment}
    return None


def retrieve_func_order(hints_obj, func):
    if hints_obj is None:
        return None
    for elem in hints_obj:
        if elem['func'] == func:
            return {'filename': elem['filename'], 'settings': elem['settings']}
    return None


def output_func_header(name, sourcecode):
    line = f"func {name}(config pkgconfigmodel.Setup) {{"
    sourcecode.append(line)


def output_func_footer(_, sourcecode):
    sourcecode.append('}')
    sourcecode.append('')


def try_parse_duration(text):
    if not isinstance(text, str):
        return None
    m = re.fullmatch(r'(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?(?:(\d+)ms)?', text)
    if not m or not any(m.groups()):
        return None
    hours = int(m.group(1) or 0)
    minutes = int(m.group(2) or 0)
    seconds = int(m.group(3) or 0)
    millis = int(m.group(4) or 0)
    parts = []
    if hours:
        parts.append('%d*time.Hour' % hours)
    if minutes:
        parts.append('%d*time.Minute' % minutes)
    if seconds:
        parts.append('%d*time.Second' % seconds)
    if millis:
        parts.append('%d*time.Millisecond' % millis)
    if not parts:
        return '0'
    return ' + '.join(parts)


def as_go_value(text):
    if not isinstance(text, str):
        text = str(text)
    text = text.replace('[', '{')
    text = text.replace(']', '}')
    text = text.replace('\'', '"')
    text = text.replace('True', 'true')
    text = text.replace('False', 'false')
    return text


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
    node = get_node(keypath, schema)
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
        platform_default = as_go_value(node['platform_default'])
        return f"GetPlatformDefault(map[string]interface{{}}{platform_default})"

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


def get_suffixes_for_pattern(pattern):
    if pattern == 'pattern_logs_config':
        return [
            'logs_dd_url',
            'dd_url',
            'additional_endpoints',
            'use_compression',
            'compression_kind',
            'zstd_compression_level',
            'compression_level',
            'batch_wait',
            'connection_reset_interval',
            'logs_no_ssl',
            'batch_max_concurrent_send',
            'batch_max_content_size',
            'batch_max_size',
            'input_chan_size',
            'sender_backoff_factor',
            'sender_backoff_base',
            'sender_backoff_max',
            'sender_recovery_interval',
            'sender_recovery_reset',
            'use_v2_api',
            'dev_mode_no_ssl',
        ]
    elif pattern == 'pattern_delegate_auth':
        return [
            'delegated_auth.org_uuid',
            'delegated_auth.refresh_interval_mins',
            'delegated_auth.provider',
            'delegated_auth.aws.region',
            'api_key',
        ]
    else:
        raise RuntimeError(f"unknown pattern: {pattern}")


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
def output_single_setting(name, kind, internal_comment, schema, target):
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
        raise RuntimeError('unknown kind: %s' % kind)

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


config_setup_func_names = [
    'initCoreAgentFull',
    'agent',
    'fleet',
    'autoscaling',
    'fips',
    'remoteconfig',
    'autoconfig',
    'containerSyspath',
    'debugging',
    'telemetry',
    'serializer',
    'aggregator',
    'serverless',
    'forwarder',
    'dogstatsd',
    'logsagent',
    'vector',
    'cloudfoundry',
    'containerd',
    'cri',
    'kubernetes',
    'podman',
    'setupAPM',
    'setupMultiRegionFailover',
    'remoteflags',
    'OTLP',
    'setupProcesses',
    'setupPrivateActionRunner',
    'anomalyDetection',
    'initMainSystemProbeConfig',
    'initCWSSystemProbeConfig',
    'initUSMSystemProbeConfig',
]


def gen_delegated_auth_map(core_schema, system_probe_schema, core_out, system_probe_out):
    """
    Constant generator: appends the delegated auth map to the relevant buffers.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    core_out            - Go source lines for the core constant file
    system_probe_out    - Go source lines for the system-probe constant file
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
        out.append("""
            type delegatedAuthConfig struct {
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

            out.append(f"""
                {{
                  apiKeyPath: "{parent_section}api_key",
                  delegatedAuthPath : "{parent_section}delegated_auth",
                  description: "{parent_section_name}",
                }},""")
        out.append("}")
        out.append("")

    emit(core_out, collect_delegated_auth_keys(core_schema))


# Ordered list of generator functions used to produce the constant files.
# Each is called with (core_schema, system_probe_schema, core_out, system_probe_out)
# and may append Go code to either output buffer.
constant_generators = [
    gen_delegated_auth_map,
]


def run_constant_codegen(core_schema, system_probe_schema, outsource_dir):
    """
    Generate the core and system-probe constant files by running each generator
    in `constant_generators` in order. Each generator receives both schemas and
    both output buffers, so it can append Go code to either file.

    core_schema         - loaded core schema object
    system_probe_schema - loaded system-probe schema object
    outsource_dir       - the directory to output source code to
    """
    header = file_header.split('\n') + constant_header.split('\n')
    core_out = list(header)
    system_probe_out = list(header)

    for generator in constant_generators:
        generator(core_schema, system_probe_schema, core_out, system_probe_out)

    for filename, sourcecode in (
        ("generated.go", core_out),
        # For now we don't have any content for system_probe.
        # ("system_probe_generated.go", system_probe_out),
    ):
        print('Output %s' % filename)
        out_filename = os.path.join(outsource_dir, filename)
        with open(out_filename, "w") as f:
            f.write(gofmt('\n'.join(sourcecode)))


def gofmt(source):
    """
    Format Go source code with gofmt and return the result.
    """
    return subprocess.run(
        ["gofmt"],
        input=source,
        capture_output=True,
        text=True,
        check=True,
    ).stdout


def run_codegen(schema, filename_filter, hints, keep_orig_order, outsource_dir):
    """
    Entry point for code generation.
    schema          - loaded schema object (dict with schema['properities'])
    filename_filter - optional function to filter output filenames (or None)
    hints           - hints object, used for func order (if keep_orig_order) and comments (always)
    keep_orig_order - bool, whether to use order from the hints object
    outsource_dir   - the directory to output source code to
    """
    target = CodeGeneratorTarget()
    if keep_orig_order:
        target.use_func_order(hints, config_setup_func_names)
    target.add_header(file_header)

    # Visitor for each setting
    def process_single_setting(keyname):
        kind = ''
        internal_comment = []
        h = retrieve_hint(hints, keyname)
        if h is not None:
            kind = h['kind']
            internal_comment = h['internal_comment']
        output_single_setting(keyname, kind, internal_comment, schema, target)

    # walk the schema to generate code
    walk_schema(schema, '', process_single_setting)
    target.flush_buffer(filename_filter)

    target.write_to_directory(outsource_dir, filename_filter)
