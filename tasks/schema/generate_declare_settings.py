import argparse
import json
import os
import re
import yaml

import tasks.schema.common_settings_analyzer as common_settings_analyzer


## WIP: does not correctly generate code yet


def read_file(filename):
    fp = open(filename, 'r')
    content = fp.read()
    fp.close()
    return content


def write_file(filename, content):
    fout = open(filename, 'w')
    fout.write(content)
    fout.close()


file_header = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup
"""


class BufferedSetting(object):
    def __init__(self, path, sourcecode):
        self.path = path
        self.sourcecode = sourcecode
        self.done = False


class SourceFileWriter():
    def __init__(self):
        self.outcode = []
        self.buffer = None
        self.result = []
        self.header_text = None

    def use_hint_order(self, hints, func_names):
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

    def flush_buffer(self, schema):
        if self.buffer is None:
            self.result_as_single_func()
            return

        sourcecode = []

        # Adapter from this 'Writer' type to concat lists
        class MyWriter:
            def __init__(self, txts):
                self.txts = txts
            def add(self, _path, more):
                self.txts = self.txts + more

        # Run over the re-ordering function list, retrieve from buffered codegen
        for name in self.reorder_func_names:
            output_func_header(name, sourcecode)
            h = self.reorder_hints.get(name)
            if not h:
                # TODO: I think this matches the loop over the !done settings
                print('[WARN] not found: %s' % name)
                continue
            for row in h:
                keyname = row[0]
                setting = self.buffer[keyname]
                self.buffer[keyname].done = True
                sourcecode = sourcecode + setting.sourcecode
            output_func_footer(name, sourcecode)

        # Afterwards: run over buffer to get everything else
        output_func_header("otherSettings", sourcecode)
        for keyname in self.buffer:
            if self.buffer[keyname].done:
                continue
            self.buffer[keyname].done = True
            sourcecode = sourcecode + self.buffer[keyname].sourcecode
        output_func_footer("otherSettings", sourcecode)
        self.result = sourcecode

    def result_as_single_func(self):
        res = self.header_text.split('\n')
        res = res + [
            'func declareSettings(config pkgconfigmodel.Setup) {'
        ]
        res = res + self.result
        res = res + [
            '}'            
        ]
        self.result = res

    def write_to_file(self, outfilename):
        with open(outfilename, "w") as f:
            f.write('\n'.join(self.result))


def join_key(path, field):
    if path == '':
        return field
    return '%s.%s' % (path, field)


def _is_node_leaf(node):
    if 'node_type' not in node:
        return True
    return node['node_type'] == 'leaf'


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
    for filename in hints_obj:
        for row in filename:
            if row[0] == keyname:
                return {
                    'kind': row[1],
                    'internal_comment': row[2]
                }
    return None


def run_generator(schema, hints, use_hints_order, outsource_dir):
    func_names = common_settings_analyzer.config_setup_func_names

    writer = SourceFileWriter()
    if use_hints_order:
        writer.use_hint_order(hints, func_names)
    writer.add_header(file_header)

    # Visitor for each setting
    def process_single_setting(keyname):
        kind = ''
        internal_comment = []
        h = retrieve_hint(hints, keyname)
        if h is not None:
            kind = h['kind']
            internal_comment = h['internal_comment']
        output_single_setting(keyname, kind, internal_comment, schema, writer)

    # walk the schema to generate code
    walk_schema(schema, '', process_single_setting)
    writer.flush_buffer(schema)

    outsource_file = os.path.join(outsource_dir, 'common_settings_generated.go')
    writer.write_to_file(outsource_file)
    print('Wrote to %s' % outsource_file)


def output_func_header(name, sourcecode):
    line = 'func %s(config pkgconfigmodel.Setup) {' % name
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


def as_go_array(text):
    if not isinstance(text, str):
        text = '%s' % text
    text = text.replace('[', '{')
    text = text.replace(']', '}')
    text = text.replace('\'', '"')
    return text


def as_go_value(text):
    if not isinstance(text, str):
        text = '%s' % text
    text = text.replace('\'', '"')
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


def retrieve_default_value(keypath, schema):
    curr = schema
    for k in keypath:
        curr = curr['properties']
        curr = curr[k]
    settingDefault = curr.get('default')
    settingType = curr.get('type')
    if settingType is None:
        return 'nil'
    if settingType == 'array':
        if curr is None or curr.get('items') is None:
            return '[]interface{}{}'
        settingItemsType = curr.get('items').get('type')
        if settingItemsType == 'object':
            return '[]map[string]interface{}%s' % (as_go_array(settingDefault),)
        return '[]%s%s' % (settingItemsType, as_go_array(settingDefault))
    elif settingType == 'boolean':
        if settingDefault:
            return 'true'
        return 'false'
    elif settingType == 'number':
        if get_golang_type_tag(curr) == 'int64':
            return 'int64(%s)' % settingDefault
        if get_golang_type_tag(curr) == 'float64':
            return 'float64(%s)' % settingDefault
        durationValue = try_parse_duration(settingDefault)
        if durationValue is not None:
            return '%s' % durationValue
        if settingDefault is None:
            return '0'
        if isinstance(settingDefault, float):
            textDefault = '%s' % settingDefault
            if '.' in textDefault:
                return '%s' % settingDefault
            return 'float64(%s.0)' % settingDefault
        if isinstance(settingDefault, int):
            return '%s' % settingDefault
    elif settingType == 'string':
        if settingDefault is None:
            return '""'
        if isinstance(settingDefault, str):
            return '"%s"' % settingDefault
    elif settingType == 'object':
        textDefault = '%s' % settingDefault
        add = curr.get('additionalProperties')
        if add is not None:
            if add.get('type') == 'string':
                return 'map[string]string%s' % as_go_value(settingDefault)
            if add.get('type') == 'array' and add.get('items').get('type') == 'string':
                return 'map[string][]string%s' % as_go_value(settingDefault)
        return 'map[string]interface{}%s' % as_go_value(settingDefault)
    raise RuntimeError('setting %s: cant handle settingType: "%s", settingDefault: "%s" of %s' % (keypath, settingType, settingDefault, type(settingDefault)))


def retrieve_envvars(keypath, schema):
    curr = schema
    for k in keypath:
        curr = curr['properties']
        curr = curr[k]
    envvars = curr.get('env_vars')
    return envvars


def output_single_setting(name, kind, internal_comment, schema, writer):
    sourcecode = []

    settingname = '"%s"' % name
    defaultval = retrieve_default_value(name.split('.'), schema)
    envsuffix = ''
    envvars = retrieve_envvars(name.split('.'), schema)
    if envvars is not None and len(envvars) > 0:
        envvars = ['"%s"' % ev for ev in envvars]
        envsuffix = ', ' + ', '.join(envvars)

    line = ''
    if kind == 'declare' or kind == 'declare_multiline' or kind == '':
        line = '\tconfig.BindEnvAndSetDefault(' + settingname + ', ' + defaultval + envsuffix + ')'
    elif kind == 'env':
        line = '\tconfig.BindEnv(' + settingname + envsuffix + ')'
    elif kind == 'known':
        line = '\tconfig.SetKnown(' + settingname + ')'
    elif kind == 'default':
        line = '\tconfig.SetDefault(' + settingname + ', ' + defaultval + ')'
    else:
        raise RuntimeError('unknown kind: %s' % kind)    

    # internal-only comments for the setting
    if internal_comment:
        for text in internal_comment.split('\n'):
            sourcecode.append('\t// %s' % text)

    # the line of code that defines the setting
    sourcecode.append(line)
    # write to our target
    writer.add(name, sourcecode)
