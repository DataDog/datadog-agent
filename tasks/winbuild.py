import glob
import os
import shutil
import tempfile
import zipfile

from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import get_version
from tasks.msi import build as build_agent_msi
from tasks.omnibus import build as omnibus_build

# Output directory for package files
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
# Omnibus stores files here, e.g. C:\opt\datadog-agent, C:\opt\datadog-installer
OPT_SOURCE_DIR = os.path.join("C:\\", "opt")
# Subdirectory of OUTPUT_PATH that holds the generated symbol-server layout
SYMBOL_STORE_DIR_NAME = "symbols"


@task
def agent_package(
    ctx,
    flavor=AgentFlavor.base.name,
    skip_deps=False,
    build_upgrade=False,
):
    # Build agent
    omnibus_build(
        ctx,
        flavor=flavor,
        skip_deps=skip_deps,
    )

    # Move the installer binary to a separate folder
    os.makedirs(os.path.join(OPT_SOURCE_DIR, "datadog-installer"))
    shutil.move(
        os.path.join(OPT_SOURCE_DIR, "datadog-agent", "datadog-installer.exe"),
        os.path.join(OPT_SOURCE_DIR, "datadog-installer"),
    )

    # Package Agent into MSI
    build_agent_msi(ctx, build_upgrade=build_upgrade)

    # Copy installer.exe to the output dir so it can be deployed as the bootstrapper
    agent_version = get_version(
        ctx,
        include_git=True,
        url_safe=True,
        include_pipeline_id=True,
    )
    shutil.copy2(
        os.path.join(OPT_SOURCE_DIR, "datadog-installer\\datadog-installer.exe"),
        os.path.join(OUTPUT_PATH, f"datadog-installer-{agent_version}-1-x86_64.exe"),
    )

    # Build the symbol-server layout from the PDBs. This must run here, in the
    # Windows build container: symstore.exe is only available during the build,
    # not in the (Linux) deploy jobs that publish to S3.
    generate_symbol_store(ctx)


def _find_symstore():
    """
    Locate symstore.exe: PATH first, then the Windows Kits Debuggers install
    locations. Returns None if not found.
    """
    found = shutil.which("symstore")
    if found:
        return found

    candidates = []
    for base in (r"C:\Program Files (x86)\Windows Kits", r"C:\Program Files\Windows Kits"):
        candidates.append(os.path.join(base, "10", "Debuggers", "x64", "symstore.exe"))
        candidates.extend(glob.glob(os.path.join(base, "*", "Debuggers", "x64", "symstore.exe")))
    for candidate in candidates:
        if os.path.exists(candidate):
            return candidate

    return None


def _extract_pdbs(debug_zips, dest_dir):
    """
    Extract every *.pdb from the given .debug.zip archives into dest_dir, one
    subdirectory per archive to avoid basename collisions across archives. Only
    PDB entries are read, so the (large) stripped-binary .debug entries are not
    paid for. Returns the list of extracted PDB paths.
    """
    pdbs = []
    for index, zip_path in enumerate(debug_zips):
        sub = os.path.join(dest_dir, str(index))
        with zipfile.ZipFile(zip_path, "r") as archive:
            for info in archive.infolist():
                if info.filename.lower().endswith(".pdb"):
                    # symstore indexes by the PDB's own GUID, not its path, so
                    # the source directory layout is irrelevant: flatten it.
                    info.filename = os.path.basename(info.filename)
                    pdbs.append(archive.extract(info, sub))
    return pdbs


def _strip_symstore_metadata(store_dir):
    """
    Remove symstore transaction bookkeeping, leaving only the indexed PDBs. A
    static S3 symbol server needs only the <pdb>/<GUID+age>/<pdb> tree; the
    000Admin folder, root marker files, and per-transaction refs.ptr files are
    symstore-internal and unused by debugger symbol lookups.
    """
    admin = os.path.join(store_dir, "000Admin")
    if os.path.isdir(admin):
        shutil.rmtree(admin)
    for name in ("pingme.txt", "server.txt", "lastid.txt"):
        path = os.path.join(store_dir, name)
        if os.path.exists(path):
            os.remove(path)
    for refs in glob.glob(os.path.join(store_dir, "**", "refs.ptr"), recursive=True):
        os.remove(refs)


@task
def generate_symbol_store(ctx, output_dir=None, store_dir=None):
    """
    Build a Windows symbol-server layout (<pdb>/<GUID+age>/<pdb>) from the agent
    PDBs, using symstore.exe.

    PDBs are read from the *.debug.zip archives the build leaves in output_dir
    (omnibus deletes the on-disk .debug tree, so the archives are the only place
    the PDBs survive). The resulting tree is portable: any runner -- including
    the Linux agent-release-management release pipeline, which has no Windows
    runners -- can publish it to S3 with `aws s3 cp --recursive`. Only PDBs are
    indexed and symstore transaction metadata is removed, so the output holds
    symbol files only.

    No-ops with a warning when symstore.exe or the debug archives are absent
    (e.g. local dev builds), so it never breaks a build.
    """
    output_dir = output_dir or OUTPUT_PATH
    store_dir = store_dir or os.path.join(output_dir, SYMBOL_STORE_DIR_NAME)

    symstore = _find_symstore()
    if not symstore:
        print(f'{color_message("Warning", Color.ORANGE)}: symstore.exe not found; skipping symbol store generation')
        return

    debug_zips = glob.glob(os.path.join(output_dir, "*.debug.zip"))
    if not debug_zips:
        print(
            f'{color_message("Warning", Color.ORANGE)}: no .debug.zip found in {output_dir}; skipping symbol store generation'
        )
        return

    with tempfile.TemporaryDirectory() as tmp:
        pdbs = _extract_pdbs(debug_zips, tmp)
        if not pdbs:
            print(
                f'{color_message("Warning", Color.ORANGE)}: no PDBs found in debug archives; skipping symbol store generation'
            )
            return

        os.makedirs(store_dir, exist_ok=True)
        # Pass the PDBs to symstore via a response file (one path per line) to
        # stay clear of command-line length limits.
        responsefile = os.path.join(tmp, "pdbs.txt")
        with open(responsefile, "w") as f:
            f.write("\n".join(pdbs))

        # /t (product) is required by symstore but, like /v, only feeds the
        # 000Admin index we discard -- so it's a throwaway literal here.
        ctx.run(f'"{symstore}" add /f "@{responsefile}" /s "{store_dir}" /t datadog-agent /o')

    _strip_symstore_metadata(store_dir)
    print(f"Symbol store generated at {store_dir}")
