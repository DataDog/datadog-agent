#!/usr/bin/env python3

import sys

from invoke.context import Context
from invoke.exceptions import UnexpectedExit

from ..go import tidy_all

if __name__ == "__main__":
    # need to tidy all modules due to the replace directives
    # might be possible to do something smarter once https://github.com/golang/go/issues/27005 is fixed
    ctx = Context()
    try:
        tidy_all(ctx)
    except UnexpectedExit:
        sys.exit(1)
