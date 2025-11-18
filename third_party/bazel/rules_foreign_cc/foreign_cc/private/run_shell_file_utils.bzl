"""A module defining some sketchy solutions for managing outputs of foreign_cc rules"""

def _created_by_script(file, script):
    """Structure to keep declared file or directory and creating script.

    Args:
        file (File): Declared file or directory
        script (str): Script that creates that file or directory

    Returns:
        struct: A struct of script info
    """
    return struct(
        file = file,
        script = script,
    )

def fictive_file_in_genroot(actions, target_name):
    """Creates a fictive file under the build root.

    This gives the possibility to address the build root in script and construct paths under it.

    Args:
        actions (ctx.actions): actions factory
        target_name (ctx.label.name): name of the current target
    """

    # we need this fictive file in the genroot to get the path of the root in the script
    empty = actions.declare_file("empty_{}.txt".format(target_name))
    return _created_by_script(
        file = empty,
        script = "##touch## $$EXT_BUILD_ROOT$$/" + empty.path,
    )

def copy_directory(actions, orig_path, copy_path):
    """Copies directory by $EXT_BUILD_ROOT/orig_path into to $EXT_BUILD_ROOT/copy_path.

    I.e. a copy of the directory is created under $EXT_BUILD_ROOT/copy_path.

    Args:
        actions: actions factory (ctx.actions)
        orig_path: path to the original directory, relative to the build root
        copy_path: target directory, relative to the build root
    """
    dir_copy = actions.declare_directory(copy_path)
    return _created_by_script(
        file = dir_copy,
        script = "\n".join([
            "##mkdirs## $$EXT_BUILD_ROOT$$/" + dir_copy.path,
            "##copy_dir_contents_to_dir## {} $$EXT_BUILD_ROOT$$/{}".format(
                orig_path,
                dir_copy.path,
            ),
        ]),
    )
