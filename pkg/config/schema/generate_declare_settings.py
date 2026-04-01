import argparse
import common_settings_analyzer
import json
import re
import yaml


## WIP: does not correctly generate code yet
##
## doesn't handle time or duration values correctly
## doesn't handle slices of data (outputs them as json, not go style slices)
## assumes everything uses BindEnvAndSetDefault
## doesn't handle function calls to bindEnvAndSetLogsConfigKeys
## doesn't handle multi-line declarations
## note: strips all comments from original code


def read_file(filename):
    fp = open(filename, 'r')
    content = fp.read()
    fp.close()
    return content


def write_file(filename, content):
    fout = open(filename, 'w')
    fout.write(content)
    fout.close()


header = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup
"""


def run_generator(schema_file, hints_file, outsource_file):
    func_names = common_settings_analyzer.config_setup_func_names

    with open(schema_file, "r") as f:
        schema = yaml.safe_load(f)
    hints = json.loads(read_file(hints_file))

    sourcecode = []
    for ln in header.split('\n'):
        sourcecode.append(ln)

    for name in func_names:
        output_func_header(name, sourcecode)
        my_hints = hints.get(name)
        if not my_hints:
            continue
        for row in my_hints:
            keyname = row[0]
            kind = row[1]
            output_single_setting(keyname, kind, schema, sourcecode)
        output_func_footer(name, sourcecode)

    with open(outsource_file, "w") as f:
        f.write('\n'.join(sourcecode))
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


def output_single_setting(name, kind, schema, sourcecode):
    settingname = '"%s"' % name
    defaultval = retrieve_default_value(name.split('.'), schema)
    envsuffix = ''
    envvars = retrieve_envvars(name.split('.'), schema)
    if envvars is not None and len(envvars) > 0:
        envvars = ['"%s"' % ev for ev in envvars]
        envsuffix = ', ' + ', '.join(envvars)

    line = ''
    if kind == 'declare' or kind == 'declare_multiline':
        line = '\tconfig.BindEnvAndSetDefault(' + settingname + ', ' + defaultval + envsuffix + ')'
    elif kind == 'env':
        line = '\tconfig.BindEnv(' + settingname + envsuffix + ')'
    elif kind == 'known':
        line = '\tconfig.SetKnown(' + settingname + ')'
    elif kind == 'default':
        line = '\tconfig.SetDefault(' + settingname + ', ' + defaultval + ')'
    else:
        raise RuntimeError('unknown kind: %s' % kind)
    sourcecode.append(line)


def main():
    argparser = argparse.ArgumentParser()
    argparser.add_argument('--outsource', dest='outsource')
    argparser.add_argument('--schema', dest='schema')
    argparser.add_argument('--hints', dest='hints')
    args = argparser.parse_args()
    run_generator(args.schema, args.hints, args.outsource)


if __name__ == '__main__':
    main()