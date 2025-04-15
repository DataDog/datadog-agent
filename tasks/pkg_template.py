import os
import shutil
import sys
from contextlib import chdir

from invoke.context import Context
from invoke.tasks import task


@task
def generate(ctx: Context):
    """
    Generate the code for the template package.
    Takes the code from the Go standard library and applies the patches.
    """
    if not shutil.which("gopatch"):
        print("gopatch could not be found, you need to run `inv -e install-tools` first...")
        sys.exit(1)

    with chdir("pkg/template"):
        _generate(ctx)


def _generate(ctx: Context):
    print(f"Generating code for Go version {get_go_version(ctx)}")

    goroot = get_goroot(ctx)
    if not goroot:
        print("Could not find Go's source code path")
        sys.exit(1)
    print(f"Using code from {goroot}")

    print("Removing previous code...")
    remove_dirs = ["text", "html", "internal"]
    for dir_name in remove_dirs:
        if os.path.exists(dir_name):
            shutil.rmtree(dir_name)

    print("Creating directories...")
    os.makedirs("internal/fmtsort", exist_ok=True)
    os.makedirs("text", exist_ok=True)
    os.makedirs("html", exist_ok=True)

    print("Copying code from Go standard library...")
    copy_go_files(f"{goroot}/src/text/template", "text")
    copy_go_files(f"{goroot}/src/html/template", "html")
    copy_go_files(f"{goroot}/src/internal/fmtsort", "internal/fmtsort")

    print("Applying patches...")
    ctx.run("git apply no-method.patch")
    ctx.run("gopatch -p imports.gopatch ./...")
    ctx.run("gopatch -p godebug.gopatch ./...")
    ctx.run("git apply types.patch")

    print("Code generation completed successfully!")


def get_go_version(ctx: Context):
    """Get the Go version."""
    result = ctx.run("go version", hide="stdout")
    assert result
    return result.stdout.strip()


def get_goroot(ctx: Context):
    """Get the GOROOT environment variable."""
    result = ctx.run("go env GOROOT", hide="stdout")
    assert result
    return result.stdout.strip()


def copy_go_files(src_dir, dest_dir):
    """Copy Go files from source to destination directory."""
    for file in os.listdir(src_dir):
        if file.endswith(".go") and not file.endswith("_test.go"):
            src_file = os.path.join(src_dir, file)
            dest_file = os.path.join(dest_dir, file)
            shutil.copy2(src_file, dest_file)
            os.chmod(dest_file, 0o664)  # ensure the file is writable
