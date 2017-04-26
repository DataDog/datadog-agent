# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import traceback

def shell():
    from config import get_version, set_win32_requests_ca_bundle_path

    set_win32_requests_ca_bundle_path()
    print """
Datadog Agent v%s - Python Shell

    """ % (get_version())
    while True:
        cmd = raw_input('>>> ')
        try:
            exec(cmd)
        except Exception as e:
            print traceback.format_exc(e)

if __name__ == "__main__":
    shell()
