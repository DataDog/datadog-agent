# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

import importlib
from functools import cached_property

import rich_click as click


class DynamicGroup(click.RichGroup):
    def __init__(self, *args, external_plugins: bool | None = None, subcommands: tuple[str], **kwargs):
        super().__init__(*args, **kwargs)

        self._external_plugins = external_plugins
        # e.g. ('dev', 'runtime', 'qa')
        self._subcommands = subcommands

    @property
    def _module(self) -> str:
        # e.g. deva.cli.env
        return self.callback.__module__

    @cached_property
    def _plugins(self) -> dict[str, str]:
        import os

        import find_exe

        plugin_prefix = self.callback.__module__.replace('deva.cli', 'deva', 1).replace('.', '-')
        plugin_prefix = f'{plugin_prefix}-'
        exe_pattern = f'^{plugin_prefix}[^-]+$'

        plugins: dict[str, str] = {}
        for executable in find_exe.with_pattern(exe_pattern):
            exe_name = os.path.splitext(os.path.basename(executable))[0]
            plugin_name = exe_name[len(plugin_prefix) :]
            plugins[plugin_name] = executable

        return plugins

    def _create_module_meta_key(self, module: str) -> str:
        return f'{module}.plugins'

    def _external_plugins_allowed(self, ctx) -> bool:
        if self._external_plugins is not None:
            return self._external_plugins

        parent_module_parts = self._module.split('.')[:-1]
        parent_key = self._create_module_meta_key('.'.join(parent_module_parts))
        return bool(ctx.meta[parent_key])

    def list_commands(self, ctx):
        commands = super().list_commands(ctx)
        commands.extend(self._subcommands)
        if self._external_plugins_allowed(ctx):
            commands.extend(self._plugins)
        return sorted(commands)

    def get_command(self, ctx, cmd_name):
        # Pass down the default setting for allowing external plugins, see:
        # https://click.palletsprojects.com/en/8.1.x/api/#click.Context.meta
        ctx.meta[self._create_module_meta_key(self._module)] = self._external_plugins_allowed(ctx)

        if cmd_name in self._subcommands:
            return self._lazy_load(cmd_name)

        if cmd_name in self._plugins:
            return _get_external_plugin_callback(cmd_name, self._plugins[cmd_name])

        return super().get_command(ctx, cmd_name)

    def _lazy_load(self, cmd_name):
        import_path = f'{self._module}.{cmd_name}'
        mod = importlib.import_module(import_path)
        cmd_object = getattr(mod, 'cmd', None)
        if not isinstance(cmd_object, click.BaseCommand):
            message = f'Unable to lazily load command: {import_path}.cmd'
            raise ValueError(message)

        return cmd_object


def _get_external_plugin_callback(cmd_name: str, executable: str):

    @click.command(
        name=cmd_name,
        short_help='[external plugin]',
        context_settings={'help_option_names': [], 'ignore_unknown_options': True},
    )
    @click.argument('args', required=True, nargs=-1)
    @click.pass_context
    def _external_plugin_callback(ctx: click.Context, args: tuple[str, ...]):
        import subprocess

        process = subprocess.run([executable, *args])
        ctx.exit(process.returncode)

    return _external_plugin_callback
