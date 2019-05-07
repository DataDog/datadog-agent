"""
Six namespaced tasks
"""
import os

from invoke import task

def get_six_path():
    here = os.path.abspath(os.path.dirname(__file__))
    return os.path.join(here, '..', 'six')

@task
def build(ctx, install_prefix=None, cmake_options=''):
    six_path = get_six_path()

    here = os.path.abspath(os.path.dirname(__file__))
    dev_path = os.path.join(here, '..', 'dev')

    cmake_args = cmake_options + " -DBUILD_DEMO:BOOL=OFF -DCMAKE_INSTALL_PREFIX:PATH={}".format(install_prefix or dev_path)

    ctx.run("cd {} && cmake {} .".format(six_path, cmake_args))
    ctx.run("make -C {}".format(six_path))

@task
def install(ctx):
    ctx.run("make -C {} install".format(get_six_path()))

@task
def test(ctx):
    ctx.run("make -C {}/test run".format(get_six_path()))

@task
def format(ctx):
    ctx.run("make -C {} clang-format".format(get_six_path()))
