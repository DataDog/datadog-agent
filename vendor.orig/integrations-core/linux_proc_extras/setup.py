# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)
from setuptools import setup
from codecs import open  # To use a consistent encoding
from os import path

HERE = path.dirname(path.abspath(__file__))

# Get version info
ABOUT = {}
with open(path.join(HERE, 'datadog_checks', 'linux_proc_extras', '__about__.py')) as f:
    exec(f.read(), ABOUT)

# Get the long description from the README file
with open(path.join(HERE, 'README.md'), encoding='utf-8') as f:
    long_description = f.read()


# Parse requirements
def get_requirements(fpath):
    with open(path.join(HERE, fpath), encoding='utf-8') as f:
        return f.readlines()


setup(
    name='datadog-linux_proc_extras',
    version=ABOUT['__version__'],
    description='The Linux Proc Extras check',
    long_description=long_description,
    keywords='datadog agent linux_proc_extras check',

    # The project's main homepage.
    url='https://github.com/DataDog/integrations-core',

    # Author details
    author='Datadog',
    author_email='packages@datadoghq.com',

    # License
    license='BSD',

    # See https://pypi.python.org/pypi?%3Aaction=list_classifiers
    classifiers=[
        'Development Status :: 5 - Production/Stable',
        'Intended Audience :: Developers',
        'Intended Audience :: System Administrators',
        'Topic :: System :: Monitoring',
        'License :: OSI Approved :: BSD License',
        'Programming Language :: Python :: 2',
        'Programming Language :: Python :: 2.7',
    ],

    # The package we're going to ship
    packages=['datadog_checks.linux_proc_extras'],

    # Run-time dependencies
    install_requires=get_requirements('requirements.in')+[
        'datadog_checks_base',
    ],

    # Testing setup and dependencies
    tests_require=get_requirements('requirements-dev.txt'),

    # Extra files to ship with the wheel package
    package_data={'datadog_checks.linux_proc_extras': ['conf.yaml.example']},
    include_package_data=True,
)
