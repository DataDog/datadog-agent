import argparse
import analyzer
import json
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
    func_names = analyzer.config_setup_func_names

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
            output_single_setting(keyname, schema, sourcecode)
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


def retrieve_default_value(keypath, schema):
    curr = schema
    for k in keypath:
        curr = curr['properties']
        curr = curr[k]
    ans = curr.get('default')
    if ans is True:
        return 'true'
    if ans is False:
        return 'false'
    if isinstance(ans, int):
        return '%s' % ans
    if isinstance(ans, str):
        return '"%s"' % ans
    return '%s' % ans


def output_single_setting(name, schema, sourcecode):
    settingname = '"%s"' % name
    # TODO: incomplete, not adding env var
    defaultval = retrieve_default_value(name.split('.'), schema)
    line = '\tconfig.BindEnvAndSetDefault(' + settingname + ', ' + defaultval + ')'
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