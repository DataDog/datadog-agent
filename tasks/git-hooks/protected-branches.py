#!/usr/bin/python3

import re
import subprocess
import sys


def main():
    try:
        local_branch = subprocess.check_output('git rev-parse --abbrev-ref HEAD', shell=True).decode('utf-8').strip()
        if local_branch == 'main':
            print("You're about to commit on main, are you sure this is what you want?")
            sys.exit(1)
        if re.fullmatch(r'^[0-9]+\.[0-9]+\.x$', local_branch):
            print("You're about to commit on a release branch, are you sure this is what you want?")
            sys.exit(1)
    except OSError as e:
        print(e)
        sys.exit(1)


if __name__ == '__main__':
    sys.exit(main())
