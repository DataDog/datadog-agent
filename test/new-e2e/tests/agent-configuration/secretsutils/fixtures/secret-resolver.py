#!/usr/bin/env python3

import json
import os
import os.path
import sys


# this script resolves secret from disk
# a single argument should be given, the directory in which to look for secrets
# for each requested handle, it will try to read a file named as the handle in the directory
def main():
    if len(sys.argv) != 2:
        raise Exception("expected a single argument being the secret directory path")

    cwd = sys.argv[1]

    content = sys.stdin.read()
    obj = json.loads(content)
    handles = obj['secrets']

    result = {}
    for h in handles:
        with open(os.path.join(cwd, h)) as reader:
            key = reader.read().strip()
        result[h] = {'value': key}

    print(json.dumps(result))


if __name__ == '__main__':
    main()
