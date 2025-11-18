""" Unit tests for shell script conversion """

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")

# buildifier: disable=bzl-visibility
load(
    "//foreign_cc/private/framework:helpers.bzl",
    "convert_shell_script_by_context",
    "do_function_call",
    "replace_var_ref",
    "split_arguments",
)

# buildifier: disable=bzl-visibility
load("//foreign_cc/private/framework/toolchains:commands.bzl", "FunctionAndCallInfo")

def _use_var_linux(varname):
    return "$" + varname

def _use_var_win(varname):
    return "%" + varname + "%"

def _replace_vars_test(ctx):
    env = unittest.begin(ctx)

    cases = {
        " $$ABC$$ ": " $ABC ",
        "$$ABC$$": "$ABC",
        "$$ABC$$/$$DEF$$": "$ABC/$DEF",
        "$$ABC$$x": "$ABCx",
        "test before $$ABC$$$ and after": "test before $ABC$ and after",
        "x$$ABC$$": "x$ABC",
    }

    shell_ = struct(
        use_var = _use_var_linux,
    )
    shell_context = struct(
        prelude = {},
        shell = shell_,
    )
    for case in cases:
        result = replace_var_ref(case, shell_context)
        asserts.equals(env, cases[case], result)

    return unittest.end(env)

def _replace_vars_win_test(ctx):
    env = unittest.begin(ctx)

    cases = {
        " $$ABC$$ ": " %ABC% ",
        "$$ABC$$": "%ABC%",
        "$$ABC$$/$$DEF$$": "%ABC%/%DEF%",
        "$$ABC$$x": "%ABC%x",
        "test before $$ABC$$$ and after": "test before %ABC%$ and after",
        "x$$ABC$$": "x%ABC%",
    }

    shell_ = struct(
        use_var = _use_var_win,
    )
    shell_context = struct(
        prelude = {},
        shell = shell_,
    )
    for case in cases:
        result = replace_var_ref(case, shell_context)
        asserts.equals(env, cases[case], result)

    return unittest.end(env)

def _funny_fun(a, b, c):
    return a + "_" + b + "_" + c

def _echo(text):
    return "echo1 " + text

def _split_arguments_test(ctx):
    env = unittest.begin(ctx)

    cases = {
        " \"\ntext\n\"": ["\"\ntext\n\""],
        " 1 2 3": ["1", "2", "3"],
        " usual \"quoted argument\"": ["usual", "\"quoted argument\""],
        "1 2": ["1", "2"],
        "var -flag1=\"redacted\" -flag2=\"redacted\"": ["var", "-flag1=\"redacted\"", "-flag2=\"redacted\""],
    }
    for case in cases:
        result = split_arguments(case)
        asserts.equals(env, cases[case], result)

    return unittest.end(env)

def _export_var(name, value):
    return "export1 {}={}".format(
        name,
        value,
    )

def _script_prelude():
    return "set -euo pipefail"

def _os_name():
    return "Fuchsia"

def _do_function_call_test(ctx):
    env = unittest.begin(ctx)

    cases = {
        "##echo## \"\ntext\n\"": "echo1 \"\ntext\n\"",
        "##script_prelude##": "set -euo pipefail",
        "##symlink_contents_to_dir## 1 2 3": "1_2_3",
        "export ROOT=\"A B C\"": "export1 ROOT=\"A B C\"",
        "export ROOT=\"ABC\"": "export1 ROOT=\"ABC\"",
        "export ROOT=ABC": "export1 ROOT=ABC",
    }
    shell_ = struct(
        symlink_contents_to_dir = _funny_fun,
        echo = _echo,
        export_var = _export_var,
        script_prelude = _script_prelude,
        os_name = _os_name,
    )
    shell_context = struct(
        prelude = {},
        shell = shell_,
    )
    for case in cases:
        result = do_function_call(case, shell_context)
        asserts.equals(env, cases[case], result)

    return unittest.end(env)

def _touch(_path):
    text = "call_touch $1"
    return FunctionAndCallInfo(text = text)

def _define_function(name, text):
    return "function " + name + "() {\n  " + text + "\n}"

def _unquote(arg):
    if arg.startswith("\"") and arg.endswith("\""):
        return arg[1:len(arg) - 1]
    return arg

def _cleanup_function(message_cleaning, message_keeping):
    text = "\n".join([
        "local ecode=$?",
        "if [ $ecode -eq 0 ]; then",
        _unquote(message_cleaning),
        "rm -rf $BUILD_TMPDIR $EXT_BUILD_DEPS",
        "else",
        "echo \"\"",
        _unquote(message_keeping),
        "echo \"\"",
        "fi",
    ])
    return FunctionAndCallInfo(text = text)

def _do_function_call_with_body_test(ctx):
    env = unittest.begin(ctx)

    cases = {
        "##cleanup_function## \"echo $$CLEANUP_MSG$$\" \"echo $$KEEP_MSG1$$ && echo $$KEEP_MSG2$$\"": {
            "call": "cleanup_function \"echo $$CLEANUP_MSG$$\" \"echo $$KEEP_MSG1$$ && echo $$KEEP_MSG2$$\"",
            "text": """function cleanup_function() {
  local ecode=$?
if [ $ecode -eq 0 ]; then
echo $$CLEANUP_MSG$$
rm -rf $BUILD_TMPDIR $EXT_BUILD_DEPS
else
echo ""
echo $$KEEP_MSG1$$ && echo $$KEEP_MSG2$$
echo ""
fi
}""",
        },
        "##touch## a/b/c": {
            "call": "touch a/b/c",
            "text": "function touch() {\n  call_touch $1\n}",
        },
    }
    shell_ = struct(
        touch = _touch,
        define_function = _define_function,
        cleanup_function = _cleanup_function,
    )
    for case in cases:
        shell_context = struct(
            prelude = {},
            shell = shell_,
        )
        result = do_function_call(case, shell_context)
        asserts.equals(env, cases[case]["call"], result)
        asserts.equals(env, cases[case]["text"], shell_context.prelude.values()[0])

    return unittest.end(env)

def _symlink_contents_to_dir(_source, _target, _replace_in_files):
    text = """local target="$2"
mkdir -p $target
local replace_in_files="${3:-}"
if [[ -f $1 ]]; then
  ##symlink_to_dir## $1 $target $replace_in_files
  return 0
fi

local children=$(find $1 -maxdepth 1 -mindepth 1)
for child in $children; do
  ##symlink_to_dir## $child $target $replace_in_files
done
"""
    return FunctionAndCallInfo(text = text)

def _symlink_to_dir(_source, _target, _replace_in_files):
    text = """local target="$2"
mkdir -p ${target}

if [[ -d $1 ]]; then
  ln -s "$1" "$target/${1##*/}"
elif [[ -f $1 ]]; then
  ln -s "$1" "$target/${1##*/}"
elif [[ -L $1 ]]; then
  cp --no-target-directory $1 ${target}
else
  echo "Can not copy $1"
fi
"""
    return FunctionAndCallInfo(text = text)

def _script_conversion_test(ctx):
    env = unittest.begin(ctx)
    script = ["##symlink_contents_to_dir## a b False"]
    expected = """function symlink_contents_to_dir() {
local target="$2"
mkdir -p $target
local replace_in_files="${3:-}"
if [[ -f $1 ]]; then
symlink_to_dir $1 $target $replace_in_files
return 0
fi

local children=$(find $1 -maxdepth 1 -mindepth 1)
for child in $children; do
symlink_to_dir $child $target $replace_in_files
done

}
function symlink_to_dir() {
local target="$2"
mkdir -p ${target}

if [[ -d $1 ]]; then
ln -s "$1" "$target/${1##*/}"
elif [[ -f $1 ]]; then
ln -s "$1" "$target/${1##*/}"
elif [[ -L $1 ]]; then
cp --no-target-directory $1 ${target}
else
echo "Can not copy $1"
fi

}
symlink_contents_to_dir a b False"""
    shell_ = struct(
        symlink_contents_to_dir = _symlink_contents_to_dir,
        symlink_to_dir = _symlink_to_dir,
        define_function = _define_function,
    )
    shell_context = struct(
        prelude = {},
        shell = shell_,
    )
    result = convert_shell_script_by_context(shell_context, script)
    asserts.equals(env, expected, result)

    return unittest.end(env)

replace_vars_test = unittest.make(_replace_vars_test)
replace_vars_win_test = unittest.make(_replace_vars_win_test)
do_function_call_test = unittest.make(_do_function_call_test)
split_arguments_test = unittest.make(_split_arguments_test)
do_function_call_with_body_test = unittest.make(_do_function_call_with_body_test)
script_conversion_test = unittest.make(_script_conversion_test)

def shell_script_conversion_suite():
    unittest.suite(
        "shell_script_conversion_suite",
        replace_vars_test,
        replace_vars_win_test,
        do_function_call_test,
        split_arguments_test,
        do_function_call_with_body_test,
        script_conversion_test,
    )
