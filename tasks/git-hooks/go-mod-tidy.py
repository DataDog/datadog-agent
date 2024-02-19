#!/usr/bin/env python3

import os
import sys

from invoke.context import Context
from invoke.exceptions import UnexpectedExit

from ..go import tidy_all

if __name__ == "__main__":
    # need to tidy all modules due to the replace directives
    # disable module lookup so that it fails as early as possible
    # might be possible to do something smarter once https://github.com/golang/go/issues/27005 is fixed
    os.environ["GOPROXY"] = "off"
    ctx = Context()
    try:
        tidy_all(ctx)
    except UnexpectedExit:
        sys.exit(1)
