"""Contains functions for conversion from intermediate multiplatform notation for defining
the shell script into the actual shell script for the concrete platform.

Notation:

1. `export <varname>=<value>`
    Define the environment variable with the name <varname> and value <value>.
    If the <value> contains the toolchain command call (see 3), the call is replaced with needed value.

2. `$$<varname>$$`
    Refer the environment variable with the name <varname>,
    i.e. this will become $<varname> on Linux/MacOS, and %<varname>% on Windows.

3. `##<funname>## <arg1> ... <argn>`
    Find the shell toolchain command Starlark method with the name <funname> for that command
    in a toolchain, and call it, passing <arg1> .. <argn>.
    (see ./shell_toolchain/commands.bzl, ./shell_toolchain/impl/linux_commands.bzl etc.)
    The arguments are space-separated; if the argument is quoted, the spaces inside the quites are
    ignored.
    ! Escaping of the quotes inside the quoted argument is not supported, as it was not needed for now.
    (quoted arguments are used for paths and never for any arbitrary string.)

The call of a shell toolchain Starlark method is performed through
//foreign_cc/private/shell_toolchain/toolchains:access.bzl; please refer there for the details.

Here what is important is that the Starlark method can also add some text (function definitions)
into a "prelude" part of the shell_context.
The resulting script is constructed from the prelude part with function definitions and
the actual translated script part.
Since function definitions can call other functions, we perform the fictive translation
of the function bodies to populate the "prelude" part of the script.
"""

load("//foreign_cc/private/framework/toolchains:access.bzl", "call_shell", "create_context")
load("//foreign_cc/private/framework/toolchains:commands.bzl", "PLATFORM_COMMANDS")

def script_extension(ctx):
    """A helper method for getting the script extension of the current foreign_cc framework toolchain

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        str: A script extension. eg: `.sh`
    """
    return call_shell(create_context(ctx), "script_extension")

def shebang(ctx):
    """A helper method for getting the shebang of the current foreign_cc framework toolchain

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        str: A shebang for a script. eg: `#!/usr/bin/env bash`
    """
    return call_shell(create_context(ctx), "shebang")

def create_function(ctx, name, text):
    return call_shell(create_context(ctx), "define_function", name, text)

def convert_shell_script(ctx, script):
    """ Converts shell script from the intermediate notation to actual schell script.

    Please see the file header for the notation description.

    Args:
        ctx: rule context
        script: the array of script strings, each string can be of multiple lines

    Returns:
        the string with the shell script for the current execution platform
    """
    return convert_shell_script_by_context(create_context(ctx), script)

# buildifier: disable=function-docstring
def convert_shell_script_by_context(shell_context, script):
    # 0. Split in lines merged fragments.
    new_script = []
    for fragment in script:
        new_script += fragment.splitlines()

    script = new_script

    # 1. Call the functions or replace export statements.
    script = [do_function_call(line, shell_context) for line in script]

    # 2. Make sure functions calls are replaced.
    # (it is known there is no deep recursion, do it only once)
    script = [do_function_call(line, shell_context) for line in script]

    # 3. Same for function bodies.
    #
    # Since we have some function bodies containing calls to other functions,
    # we need to replace calls to the new functions and add the text
    # of those functions to shell_context.prelude several times,
    # and 4 times is enough for our toolchain.
    # Example of such function: 'symlink_contents_to_dir'.
    processed_prelude = {}
    for _ in range(1, 4):
        for key in shell_context.prelude.keys():
            text = shell_context.prelude[key]
            lines = text.splitlines()
            replaced = "\n".join([
                do_function_call(line.strip(" "), shell_context)
                for line in lines
            ])
            processed_prelude[key] = replaced

    for key in processed_prelude.keys():
        shell_context.prelude[key] = processed_prelude[key]

    script = shell_context.prelude.values() + script

    # 4. replace all variable references
    script = [replace_var_ref(line, shell_context) for line in script]

    result = "\n".join(script)
    return result

# buildifier: disable=function-docstring
def replace_var_ref(text, shell_context):
    parts = []
    current = text

    for _ in range(2147483647):
        (before, varname, after) = extract_wrapped(current, "$$")
        if not varname:
            parts.append(current)
            break
        parts.append(before)
        parts.append(shell_context.shell.use_var(varname))
        current = after

    return "".join(parts)

def escape_dquote_bash(text):
    """ Escape double quotes in flag lists for use in bash strings. """

    # We use a starlark raw string to prevent the need to escape backslashes for starlark as well.
    return text.replace('"', r'\\\\\"')

# buildifier: disable=function-docstring
def replace_exports(text, shell_context):
    text = text.strip(" ")
    (varname, separator, value) = text.partition("=")
    if not separator:
        fail("Wrong export declaration")

    (funname, after) = get_function_name(value.strip(" "))

    if funname:
        value = call_shell(shell_context, funname, *split_arguments(after.strip(" ")))

    return call_shell(shell_context, "export_var", varname, value)

# buildifier: disable=function-docstring
def get_function_name(text):
    (funname, _, after) = text.partition(" ")

    if funname == "export":
        return (funname, after)

    (before, funname_extracted, after_extracted) = extract_wrapped(funname, "##", "##")

    if funname_extracted and PLATFORM_COMMANDS.get(funname_extracted):
        if len(before) > 0 or len(after_extracted) > 0:
            fail("Something wrong with the shell command call notation: " + text)
        return (funname_extracted, after)

    return (None, None)

# buildifier: disable=function-docstring
def extract_wrapped(text, prefix, postfix = None):
    postfix = postfix or prefix
    (before, separator, after) = text.partition(prefix)
    if not separator or not after:
        return (text, None, None)
    (varname, separator2, after2) = after.partition(postfix)
    if not separator2:
        fail("Variable or function name is not marked correctly in fragment: {}".format(text))
    return (before, varname, after2)

# buildifier: disable=function-docstring
def do_function_call(text, shell_context):
    (funname, after) = get_function_name(text.strip(" "))
    if not funname:
        return text

    if funname == "export":
        return replace_exports(after, shell_context)

    arguments = split_arguments(after.strip(" ")) if after else []

    return call_shell(shell_context, funname, *arguments)

# buildifier: disable=function-docstring
def split_arguments(text):
    parts = []
    current = text.strip(" ")

    for _ in range(1, 2147483647):
        current = current.strip(" ")
        if not current:
            break

        # we are ignoring escaped quotes
        s_quote = current.find("\"")
        if s_quote < 0:
            for e in current.split(" "):
                if len(e) > 0:
                    parts.append(e)
            break

        e_quote = current.find("\"", s_quote + 1)
        if e_quote < 0:
            fail("Incorrect quoting in fragment: {}".format(current))

        # backtrack to first space, from here to e_quote is a token
        e_before = current.rfind(" ", 0, s_quote)
        if e_before >= 0:
            before = current[0:e_before].strip(" ")
            parts += before.split(" ")
        parts.append(current[e_before + 1:e_quote + 1])
        current = current[e_quote + 1:]

    return parts
