#!/opt/stackstate-agent/embedded/bin/python

import argparse
import json
import os.path
import sys

MAX_FILE_SIZE_BYTES = 1024


def list_secret_names(input_json):
    query = json.loads(input_json)
    version = query["version"].split(".")
    if version[0] != "1":
        raise ValueError("incompatible protocol version {}".format(query["version"]))

    names = query["secrets"]
    if type(names) is not list:
        raise ValueError("the secrets field should be an array: {}".format(names))

    return names


def read_file(root_folder, filename):
    path = os.path.join(root_folder, filename)
    realpath = os.path.realpath(path)

    if not realpath.startswith(root_folder):
        raise ValueError("file {} is outside of the specified folder {}".format(realpath, root_folder))

    with open(realpath, "r") as f:
        return f.read(MAX_FILE_SIZE_BYTES)


def is_valid_folder(arg):
    if not os.path.isdir(arg):
        raise argparse.ArgumentTypeError('The folder {} does not exist'.format(arg))
    else:
        return arg


if __name__ == '__main__':
    parser = argparse.ArgumentParser(
        description='''
            Helper script to extract values from secret files.
            It implements the secrets protocol 1.0 as specified in Agent 6.3.0.
            To avoid leaking information, this script refuses to read files
            outside of its specified root folder.
        ''',
        epilog='''
            See https://github.com/DataDog/datadog-agent/blob/6.4.x/docs/agent/secrets.md
            for more information on the secrets protocol.
        '''
    )
    parser.add_argument(
        "root_folder",
        help="folder where secrets are mounted, eg. /run/secrets",
        default="/run/secrets",
        type=is_valid_folder
    )

    args = parser.parse_args()
    input_json = sys.stdin.read()
    try:
        secret_names = list_secret_names(input_json)
    except ValueError as e:
        sys.exit('Cannot parse input: ' + str(e))

    output = {}
    for s in secret_names:
        try:
            contents = read_file(args.root_folder, s)
            output[s] = {"value": contents}
        except (ValueError, IOError) as e:
            output[s] = {"error": str(e)}

    print json.dumps(output)
