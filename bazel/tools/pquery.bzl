"""Starlark script to nicely format providers of a target.

Usage:
    bazel cquery --output starlark --starlark:file ${PATH_TO_THIS_FILE} //path/to/pkg:target

If you get "ERROR: --starlark:file :: Unrecognized option: --starlark:file", make sure you are
using `bazel cquery` **not** `bazel query`.

If you get "Error: Starlark computation cancelled: too many steps", you can try adding
`--max_computation_steps=9223372036854775807` to raise the limit to its maximum value.

This script attempts to print all the providers for a given target in a format similar to how
they would actually be written in Starlark such as:

```starlark
FileProvider = file_provider(
    files_to_build = depset([
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/prerender_dep.d.ts>,
        <source file node_modules/typescript/lib/protocol.d.ts>,
        <source file node_modules/typescript/lib/tsserverlibrary.d.ts>,
        <source file node_modules/typescript/lib/typescript.d.ts>,
        <source file node_modules/typescript/lib/typescriptServices.d.ts>,
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/component.d.ts>,
    ]),
),
OutputGroupInfo = OutputGroupInfo(
    _validation = depset([
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/component_resources>,
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/resources>,
    ]),
),
@rules_nodejs//nodejs/private/providers:declaration_info.bzl%DeclarationInfo = struct(
    declarations = depset([
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/component.d.ts>,
    ]),
    transitive_declarations = depset([
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/prerender_dep.d.ts>,
        <source file node_modules/typescript/lib/protocol.d.ts>,
        <source file node_modules/typescript/lib/tsserverlibrary.d.ts>,
        <source file node_modules/typescript/lib/typescript.d.ts>,
        <source file node_modules/typescript/lib/typescriptServices.d.ts>,
        <generated file packages/rules_prerender/prerender_component_publish_files_testdata/component.d.ts>,
    ]),
    type_blocklisted_declarations = depset([]),
),
```

This script attempts to handle all Starlark data types, but likely some have slipped through
the cracks. It is written in a far more complicated way than it should be because Starlark
doesn't support recursion, which means that operating on tree-like structures (such as
arbitrarily nested lists, providers, and dictionaries) is significantly more difficult than
it normally would.

This works by manually managing what would normally be the call stack. Functions don't
recursively call each other, but instead process an item (such as a list) and append any new
items discovered (all the elements of the list) to the stack. The main `format()` function is
then responsible for processing all the new items on the stack.

The `_raw()` function generates a special "raw" string value which is just appended to the
output, while every other value on the stack gets processed by the appropriate function which
appends `_raw()` strings and sub-values onto the stack to get subsequently processed.


Credits:

Special thanks to `lbcjbb` from the Bazel Slack for fixing booleans and adding tuple support.

This tool is from https://gist.github.com/dgp1130/a26706cf3a85a6dcf7484591ddff41ba.
It contained no license or copyright notices. But we give credit to Douglas Parker.

It may contain Datadog local changes.

"""

def format(target):
    """Entry point which formats the providers for the given target.

    Args:
      target: a target

    Returns:
      string
    """
    target_providers = providers(target)
    if not target_providers:
        return "No providers for target - %s" % target

    stack = [_make_item(
        name = name,
        value = provider,
        indent = 0,
    ) for (name, provider) in target_providers.items()[::-1]]

    # No way to infinite loop (`while` not supported, no `Infinity` symbol,
    # `range(0, 1, -1)` just doesn't work), so instead we loop with the largest
    # available number of times. Starlark requires the value be a 32-bit signed
    # integer, so `2 ^ 31 - 1` is the best we can do.
    result = ""
    for i in range(_pow(2, 31) - 1):  # buildifier: disable=unused-variable
        if not stack:
            break

        item = stack.pop()
        if "bazel_raw_string" in item:
            result += item["value"]
            continue

        _process(stack, item)

    return result

def _push(stack, item_or_items):
    if type(item_or_items) != "list":
        items = [item_or_items]
    else:
        items = item_or_items

    stack.extend(items[::-1])

def _process(stack, item):
    # Named value.
    if "name" in item:
        _push(stack, [
            _raw("%s%s = " % (_indent(item["indent"]), item["name"])) if not item["dict"] else _raw("%s\"%s\": " % (_indent(item["indent"]), item["name"])),
            _drop_name(item),
            _raw(",\n"),
        ])
        return

    # Unnamed value.
    kind = type(item["value"])
    if item["value"] == None:
        _process_none(stack, item)
    elif kind == "string":
        _process_string(stack, item)
    elif kind == "bool":
        _process_boolean(stack, item)
    elif kind == "number":
        _process_number(stack, item)
    elif kind == "list":
        _process_list(stack, item)
    elif kind == "tuple":
        _process_tuple(stack, item)
    elif kind == "dict":
        _process_dict(stack, item)
    elif kind == "struct":
        _process_struct(stack, item)
    elif kind == "depset":
        _process_depset(stack, item)
    elif kind == "File":
        _process_file(stack, item)
    elif kind == "Label":
        _process_label(stack, item)
    else:
        # Assume any other types are providers and "struct-like".
        _process_struct_like(stack, item)

# buildifier: disable=unused-variable
def _process_none(stack, item):
    _push(stack, [_raw("None")])

def _process_string(stack, item):
    formatted = "\"\"\"%s\"\"\"" % item["value"] if "\n" in item["value"] else "\"%s\"" % item["value"]
    _push(stack, [
        _raw("%s%s" % (_indent(item["indent"]), formatted)),
    ])

def _process_boolean(stack, item):
    _push(stack, [_raw(str(item["value"]))])

def _process_number(stack, item):
    _push(stack, [_raw(str(item["value"]))])

def _process_list(stack, list):
    if not list["value"]:
        _push(stack, [_raw("[]")])
    else:
        items = [_make_item(
            value = value,
            indent = list["indent"] + 4,
        ) for value in list["value"]]
        _push(
            stack,
            [_raw("[\n%s" % _indent(list["indent"] + 4))] +
            _join(_raw(",\n%s" % _indent(list["indent"] + 4)), items) +
            [_raw(",\n%s]" % _indent(list["indent"]))],
        )

def _process_tuple(stack, item):
    if not item["value"]:
        _push(stack, [_raw("()")])
    else:
        items = [_make_item(
            value = value,
            indent = item["indent"] + 4,
        ) for value in item["value"]]
        _push(
            stack,
            [_raw("(\n%s" % _indent(item["indent"] + 4))] +
            _join(_raw(",\n%s" % _indent(item["indent"] + 4)), items) +
            [_raw(",\n%s)" % _indent(item["indent"]))],
        )

def _process_dict(stack, dict):
    if not dict["value"]:
        _push(stack, [_raw("{}")])
    else:
        _push(
            stack,
            [_raw("{\n")] +
            [_make_item(
                name = key,
                value = value,
                indent = dict["indent"] + 4,
                dict = True,
            ) for (key, value) in dict["value"].items()] +
            [_raw("%s}" % _indent(dict["indent"]))],
        )

def _process_struct(stack, struct):
    _process_struct_like(stack, struct)

def _process_struct_like(stack, struct_like):
    _push(
        stack,
        [_raw("%s(\n" % type(struct_like["value"]))] +
        [_make_item(
            name = key,
            value = value,
            indent = struct_like["indent"] + 4,
        ) for (key, value) in _items(struct_like["value"])] +
        [_raw("%s)" % _indent(struct_like["indent"]))],
    )

def _process_depset(stack, depset):
    _push(stack, [
        _raw("depset("),
        _make_item(
            value = depset["value"].to_list(),
            indent = depset["indent"],
        ),
        _raw(")"),
    ])

def _process_file(stack, file):
    _push(stack, [_raw(str(file["value"]))])

def _process_label(stack, label):
    _push(stack, [_raw("Label(%s)" % str(label["value"]))])

def _make_item(name = None, value = None, indent = 0, dict = False):
    if not name and dict:
        fail("Must be a named item to be used as a dictionary entry.")

    if name:
        return {
            "name": name,
            "value": value,
            "indent": indent,
            "dict": dict,
        }
    else:
        return {
            "value": value,
            "indent": indent,
            "dict": dict,
        }

def _drop_name(item):
    return {
        "value": item["value"],
        "indent": item["indent"],
    }

def _raw(string):
    return {
        "bazel_raw_string": True,
        "value": string,
    }

def _indent(spaces):
    return " " * spaces

def _items(obj):
    keys = dir(obj)
    result = []
    for key in keys:
        value = getattr(obj, key)
        if type(value) != "builtin_function_or_method":
            result.append((key, value))
    return result

def _join(joiner, list):
    if len(list) == 0 or len(list) == 1:
        return list

    result = [list[0]]
    rest = list[1:]
    for item in rest:
        result.append(joiner)
        result.append(item)
    return result

def _pow(x, y):
    product = 1
    for _ in range(y):
        product *= x
    return product
