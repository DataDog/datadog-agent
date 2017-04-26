# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
from datetime import date
import os
import sys

# 3p
from setuptools import setup

# project
from config import get_version

# Extra arguments to pass to the setup function
extra_args = {}

# Prereqs of the build. Won't get installed when deploying the egg.
setup_requires = []

# Prereqs of the install. Will install when deploying the egg.
install_requires = []

# Modified on mac
app_name = 'datadog-agent'
# plist (used only on mac)
plist = None

if sys.platform == 'win32':
    # noqa for flake8, these imports are probably here to force packaging of these modules
    import py2exe  # noqa

    # windows-specific deps
    install_requires.append('pywin32==217')

    # Modules to force-include in the exe
    include_modules = [
        # 3p
        'psutil',
        'servicemanager',
        'subprocess',
        'win32api',
        'win32event',
        'win32service',
        'win32serviceutil',
    ]

    class Target(object):
        def __init__(self, **kw):
            self.__dict__.update(kw)
            self.version = get_version()
            self.company_name = 'Datadog, Inc.'
            self.copyright = 'Copyright {} Datadog, Inc.'.format(date.today().year)
            self.cmdline_style = 'pywin32'

    extra_args = {
        'options': {
            'py2exe': {
                'includes': ','.join(include_modules),
                'optimize': 0,
                'compressed': True,
                'bundle_files': 3,
                'excludes': ['numpy'],
                'dll_excludes': ["IPHLPAPI.DLL", "NSI.dll",  "WINNSI.DLL",  "WTSAPI32.dll", "crypt32.dll"],
                'ascii': False,
            },
        },
        'windows': [{'script': 'gui.py',
                     'dest_base': "agent-manager",
                     'uac_info': "requireAdministrator", # The manager needs to be administrator to stop/start the service
                     'icon_resources': [(1, r"dd_agent_win_256.ico")],
                     }],
        'data_files': [],
    }

elif sys.platform == 'darwin':
    app_name = 'Datadog Agent'

    from plistlib import Plist
    plist = Plist.fromFile(os.path.dirname(os.path.realpath(__file__)) + '/packaging/Info.plist')
    plist.update(dict(
        CFBundleGetInfoString="{0}, Copyright (c) 2009-{1}, Datadog Inc.".format(
            get_version(), date.today().year),
        CFBundleVersion=get_version()
    ))

    extra_args = {
        'app': ['gui.py'],
        'data_files': [
            'images',
            'status.html',
        ],
        'options': {
            'py2app': {
                'optimize': 0,
                'iconfile': 'packaging/Agent.icns',
                'plist': plist
            }
        }
    }


setup(
    name=app_name,
    version=get_version(),
    description="DevOps' best friend",
    author='DataDog',
    author_email='dev@datadoghq.com',
    url='http://www.datadoghq.com',
    install_requires=install_requires,
    setup_requires=setup_requires,
    packages=[],
    include_package_data=True,
    test_suite='nose.collector',
    zip_safe=False,
    **extra_args
)
