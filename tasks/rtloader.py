"""
RtLoader namespaced tasks
"""
import os

from invoke import task

def get_rtloader_path():
    here = os.path.abspath(os.path.dirname(__file__))
    return os.path.join(here, '..', 'rtloader')

@task
def build(ctx, install_prefix=None, python_runtimes=None, cmake_options='', arch="x64"):
    rtloader_path = get_rtloader_path()

    here = os.path.abspath(os.path.dirname(__file__))
    dev_path = os.path.join(here, '..', 'dev')

    cmake_args = cmake_options + " -DBUILD_DEMO:BOOL=OFF -DCMAKE_INSTALL_PREFIX:PATH={}".format(install_prefix or dev_path)

    python_runtimes = python_runtimes or os.environ.get("PYTHON_RUNTIMES") or "2"
    python_runtimes = python_runtimes.split(',')
    if '2' not in python_runtimes:
        cmake_args += " -DDISABLE_PYTHON2=ON "
    if '3' not in python_runtimes:
        cmake_args += " -DDISABLE_PYTHON3=ON "

    if arch == "x86":
        cmake_args += " -DARCH_I386=ON"

    ctx.run("cd {} && cmake {} .".format(rtloader_path, cmake_args))
    ctx.run("make -C {}".format(rtloader_path))

@task
def install(ctx):
    ctx.run("make -C {} install".format(get_rtloader_path()))

@task
def test(ctx):
    ctx.run("make -C {}/test run".format(get_rtloader_path()))

@task
def format(ctx):
    ctx.run("make -C {} clang-format".format(get_rtloader_path()))
