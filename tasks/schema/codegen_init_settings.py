import os
import re

file_header = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup
"""


class BufferedSetting:
    def __init__(self, path, sourcecode):
        self.path = path
        self.sourcecode = sourcecode
        self.done = False


class CodeGeneratorTarget:
    def __init__(self):
        self.buffer = None
        self.result = []
        self.header_text = None
        self.filesystem = None

    def use_func_order(self, hints, func_names):
        self.reorder_hints = hints
        self.reorder_func_names = func_names
        self.buffer = {}

    def add_header(self, text):
        self.header_text = text

    def add(self, path, sourcecode):
        if self.buffer is None:
            self.result = self.result + sourcecode
            return
        self.buffer[path] = BufferedSetting(path, sourcecode)

    def flush_buffer(self):
        # Without the --keep-orig-order flag, output everything in 1 function
        if self.buffer is None:
            self.result_as_single_func()
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

            # Get filename to write to, add header if its empty
            need_import_statements = False
            (
                filename,
                settings,
            ) = h['filename'], h['settings']
            if filename not in self.filesystem:
                self.filesystem[filename] = self.header_text.split('\n')
                need_import_statements = True

            # Determine if the target file needs to import pkgconfighelper
            need_pkgconfighelper = False
            for row in settings:
                keyname = row[0]
                setting = self.buffer[keyname]
                for line in setting.sourcecode:
                    if 'pkgconfighelper.' in line:
                        need_pkgconfighelper = True

            # Imports section
            if need_import_statements:
                self.filesystem[filename] += self._add_imports(need_pkgconfighelper)

            # Create the function, declare all settings in it
            sourcecode = self.filesystem[filename]
            output_func_header(funcname, sourcecode)
            for row in settings:
                keyname = row[0]
                setting = self.buffer[keyname]
                self.buffer[keyname].done = True
                sourcecode = sourcecode + setting.sourcecode
            output_func_footer(funcname, sourcecode)
            self.filesystem[filename] = sourcecode

        # Afterwards: run over buffer to get everything else
        sourcecode = []
        output_func_header("otherSettings", sourcecode)
        for keyname in self.buffer:
            if self.buffer[keyname].done:
                continue
            self.buffer[keyname].done = True
            sourcecode = sourcecode + self.buffer[keyname].sourcecode
        output_func_footer("otherSettings", sourcecode)
        self.filesystem['other_settings.go'] = sourcecode

    def _add_imports(self, need_pkgconfighelper):
        sourcecode = ['import (']
        if need_pkgconfighelper:
            sourcecode += ['\tpkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"']
        sourcecode += ['\tpkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"']
        sourcecode += [')', '']
        return sourcecode

    def result_as_single_func(self):
        res = self.header_text.split('\n')
        res += self._add_imports(False)
        res += ['func declareSettings(config pkgconfigmodel.Setup) {']
        res += self.result
        res += ['}']
        self.filesystem = {'settings.go': res}

    def write_to_directory(self, out_dir):
        for filename in self.filesystem:
            out_filename = os.path.join(out_dir, filename)
            with open(out_filename, "w") as f:
                f.write('\n'.join(self.filesystem[filename]))


def join_key(path, field):
    if path == '':
        return field
    return f"{path}.{field}"


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
        add = curr.get('additionalProperties')
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
    return f"{dict_to_gotype(node)}{setting_default}"


def retrieve_method_to_declare(keypath, schema):
    node = get_node(keypath, schema)
    tags = node.get('tags')
    if tags:
        if 'no-env' in tags:
            return 'SetDefault'
        if 'TODO:fix-no-default' in tags:
            return 'BindEnv'
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
def output_single_setting(name, kind, internal_comment, schema, target):
    sourcecode = []

    settingname = '"%s"' % name
    defaultval = retrieve_default_value(name.split('.'), schema)
    envsuffix = ''
    envvars = retrieve_envvars(name.split('.'), schema)
    if envvars is not None and len(envvars) > 0:
        envvars = ['"%s"' % ev for ev in envvars]
        envsuffix = ', ' + ', '.join(envvars)

    env_parser = retrieve_env_parser(name.split('.'), schema)
    if env_parser:
        def get_vartype():
            node = get_node(name.split('.'), schema)
            return to_vartype(node, '{}')
        line = env_parser_to_func_call(name, env_parser, get_vartype)
        sourcecode.append(line)

    # internal-only comments for the setting
    if internal_comment:
        for text in internal_comment.split('\n'):
            sourcecode.append('\t// %s' % text)

    method_name = retrieve_method_to_declare(name.split('.'), schema)
    if method_name == 'BindEnvAndSetDefault':
        line = f"\tconfig.BindEnvAndSetDefault({settingname}, {defaultval}{envsuffix})"
    elif method_name == 'BindEnv':
        line = f"\tconfig.BindEnv({settingname}{envsuffix})"
    elif method_name == 'SetDefault':
        line = f"\tconfig.SetDefault({settingname}, {defaultval})"
    else:
        raise RuntimeError('unknown kind: %s' % kind)

    # the line of code that defines the setting
    sourcecode.append(line)
    # write to our target
    target.add(name, sourcecode)


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
    'OTLP',
    'setupProcesses',
    'platformCWSConfig',
    'initCWSSystemProbeConfig',
    'initUSMSystemProbeConfig',
    'InitSystemProbeConfig',
]


def run_codegen(schema, hints, keep_orig_order, outsource_dir):
    """
    Entry point for code generation.
    schema          - a loaded schema object, a dict with schema['properities']
    hints           - hints object
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
    target.flush_buffer()

    target.write_to_directory(outsource_dir)
    print(f"Wrote to {outsource_dir}")
