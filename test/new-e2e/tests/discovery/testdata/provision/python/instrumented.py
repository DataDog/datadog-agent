#!/usr/bin/env python3

import ddtrace.auto  # type: ignore # noqa: F401
import server

if __name__ == '__main__':
    server.run()
