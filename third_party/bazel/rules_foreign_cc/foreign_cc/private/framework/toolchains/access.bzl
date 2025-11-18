"""A module defining functionality for invoking toolchain commands"""

load(":commands.bzl", "FunctionAndCallInfo", "PLATFORM_COMMANDS")

_function_and_call_type = type(FunctionAndCallInfo(text = ""))

def create_context(ctx):
    """Generate a 'context' object from the current foreign_cc framework toolchain

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        struct: A foreign_cc framework context:
            - shell (struct): All commands for the current toolchain
            - prelude (dict): A cache for rendered functions
    """
    return struct(
        shell = ctx.toolchains[Label("//foreign_cc/private/framework:shell_toolchain")].commands,
        prelude = {},
    )

def call_shell(shell_context, method_, *args):
    """Calls the 'method_' shell command from the toolchain.

    Checks the number and types of passed arguments.
    If the command returns the resulting text wrapped into `FunctionAndCallInfo` provider,
    puts the text of the function into the 'prelude' dictionary in the 'shell_context',
    and returns only the call of that function.

    Args:
        shell_context (struct): A shell_context created by `create_context`
        method_ (str): The command to invoke from teh shell context's commands
        *args: Optinal arguments to accomponany `method_`

    Returns:
        str: the rendered command
    """
    check_argument_types(method_, args)

    func_ = getattr(shell_context.shell, method_)
    result = func_(*args)

    if type(result) == _function_and_call_type:
        # Cache the rendered function for use when rendering
        # the full scrupt in `convert_shell_script_by_context`
        if not shell_context.prelude.get(method_):
            shell_context.prelude[method_] = shell_context.shell.define_function(method_, result.text)

        # use provided method of calling a defined function or use default
        if hasattr(result, "call"):
            return result.call

        # TODO: This doesn't play well with arguments such as `foo="bar"`.
        return " ".join([method_] + [_wrap_if_needed(str(arg)) for arg in args])

    return result

def _quoted(arg):
    """Check if arguments to framework functions are wrapped in quotes.

    Args:
        arg (str): The target argument

    Returns:
        bool: True if `arg` is quoted
    """
    return arg.startswith("\"") and arg.endswith("\"")

def _wrap_if_needed(arg):
    """Ensure arguments to framework functions are wrapped in quotes.

    Args:
        arg (str): The target argument

    Returns:
        str: `arg` guaranteed to be wrapped by double quotes (`"`)
    """
    if arg.find(" ") >= 0 and not _quoted(arg):
        return "\"" + arg + "\""
    return arg

def check_argument_types(method_, args_list):
    """Check a method's argument types

    Args:
        method_ (str): The target method
        args_list (list): A list of arguments to check
    """
    descriptor = PLATFORM_COMMANDS[method_]
    args_info = descriptor.arguments

    if len(args_list) != len(args_info):
        fail("Wrong number ({}) of arguments ({}) in a call to '{}'".format(
            len(args_list),
            str(args_list),
            method_,
        ))

    for idx in range(0, len(args_list)):
        if type(args_list[idx]) != args_info[idx].data_type:
            fail("Wrong argument '{}' type: '{}'".format(args_info[idx].name, type(args_list[idx])))
