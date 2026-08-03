import glob
import os
import shutil
import struct
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

    # Build the symbol-server layout from the PDBs. Runs here, in the Windows
    # build job, because the PDBs survive only inside the .debug.zip the build
    # produces (omnibus deletes the on-disk .debug tree); the (Linux) deploy
    # jobs then publish the prebuilt layout to S3.
    generate_symbol_store(ctx)


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
                    # The store path is derived from the PDB's own GUID, not its
                    # source location, so the directory layout is irrelevant:
                    # flatten to the basename.
                    info.filename = os.path.basename(info.filename)
                    pdbs.append(archive.extract(info, sub))
    return pdbs


def _format_index_key(guid, age):
    """
    Format a 16-byte PDB GUID + age into the symbol-server directory key:
    {Data1:08X}{Data2:04X}{Data3:04X}{Data4 bytes} (the first three GUID fields
    are little-endian) followed by the age in uppercase hex.
    """
    d1, d2, d3 = struct.unpack_from("<IHH", guid, 0)
    tail = "".join(f"{b:02X}" for b in guid[8:16])
    return f"{d1:08X}{d2:04X}{d3:04X}{tail}{age:X}"


def _symbol_index_key(pdb_path):
    """
    Compute the symbol-server index key (`<GUID><age>`) for a PDB by reading its
    MSF "PDB Info" stream (stream 1: version, signature, age, 16-byte GUID).
    This is the same key debuggers derive from the binary's RSDS debug-directory
    record, so symsrv finds the PDB at `<pdb>\\<key>\\<pdb>`.

    We compute the key ourselves rather than shelling out to symstore.exe. This
    was originally written because symstore failed to index the agent PDBs with
    "unsupported format, ErrorLevel 11" -- but that turned out to be the
    binutils/ld 2.43 `--pdb` bug (cdb and xperf can't read those PDBs either),
    and ld >= 2.44 fixes all of them. We keep the in-house indexer anyway: it's
    straightforward, it works, and it avoids having to discard the extra
    index/transaction files (000Admin, etc.) symstore would create alongside the
    store.

    Reads only the blocks it needs (superblock, stream directory, PDB Info
    stream) by seeking, never loading the whole file -- PDBs can be hundreds of
    MB and the data we need is a tiny, scattered subset.
    """
    with open(pdb_path, "rb") as f:

        def read_block(idx):
            f.seek(idx * block_size)
            return f.read(block_size)

        # MSF superblock: block size and where the stream directory lives.
        f.seek(32)
        block_size, _free, _nblocks, num_dir_bytes, _unk, block_map_addr = struct.unpack("<IIIIII", f.read(24))

        # The directory's own blocks are listed in a single block at
        # block_map_addr; concatenating them yields the stream directory.
        num_dir_blocks = (num_dir_bytes + block_size - 1) // block_size
        dir_blocks = struct.unpack_from(f"<{num_dir_blocks}I", read_block(block_map_addr), 0)
        directory = b"".join(read_block(b) for b in dir_blocks)[:num_dir_bytes]

        # Directory: u32 numStreams, u32 sizes[numStreams], then each stream's
        # block list. Walk to stream 1 (the PDB Info stream).
        num_streams = struct.unpack_from("<I", directory, 0)[0]
        sizes = struct.unpack_from(f"<{num_streams}I", directory, 4)
        off = 4 + 4 * num_streams
        info = b""
        for i, size in enumerate(sizes):
            nblocks = 0 if size in (0, 0xFFFFFFFF) else (size + block_size - 1) // block_size
            blocks = struct.unpack_from(f"<{nblocks}I", directory, off)
            off += 4 * nblocks
            if i == 1:
                info = b"".join(read_block(b) for b in blocks)[:size]
                break

    age = struct.unpack_from("<I", info, 8)[0]
    guid = info[12:28]
    return _format_index_key(guid, age)


@task
def generate_symbol_store(ctx, output_dir=None, store_dir=None):
    """
    Build a Windows symbol-server layout (`<pdb>\\<GUID><age>\\<pdb>`) from the
    agent PDBs, indexing them in pure Python (see _symbol_index_key for why we
    don't use symstore.exe).

    PDBs are read from the *.debug.zip archives the build leaves in output_dir
    (omnibus deletes the on-disk .debug tree, so the archives are the only place
    the PDBs survive). The resulting tree is portable: any runner -- including
    the Linux agent-release-management release pipeline, which has no Windows
    runners -- can publish it to S3 with `aws s3 cp --recursive`.

    No-ops with a warning when the debug archives are absent (e.g. local dev
    builds), so it never breaks a build.
    """
    output_dir = output_dir or OUTPUT_PATH
    store_dir = store_dir or os.path.join(output_dir, SYMBOL_STORE_DIR_NAME)

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

        for pdb in pdbs:
            name = os.path.basename(pdb)
            dest = os.path.join(store_dir, name, _symbol_index_key(pdb))
            os.makedirs(dest, exist_ok=True)
            shutil.copy2(pdb, os.path.join(dest, name))

    print(f"Symbol store generated at {store_dir}")
