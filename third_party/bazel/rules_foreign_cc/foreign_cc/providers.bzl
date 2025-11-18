""" A module containing all public facing providers """

ForeignCcDepsInfo = provider(
    doc = """Provider to pass transitive information about external libraries.""",
    fields = {
        "artifacts": "Depset of ForeignCcArtifactInfo",
    },
)

ForeignCcArtifactInfo = provider(
    doc = """Groups information about the external library install directory,
and relative bin, include and lib directories.

Serves to pass transitive information about externally built artifacts up the dependency chain.

Can not be used as a top-level provider.
Instances of ForeignCcArtifactInfo are encapsulated in a depset [ForeignCcDepsInfo::artifacts](#ForeignCcDepsInfo-artifacts).""",
    fields = {
        "bin_dir_name": "Bin directory, relative to install directory",
        "dll_dir_name": "DLL directory, relative to install directory",
        "gen_dir": "Install directory",
        "include_dir_name": "Include directory, relative to install directory",
        "pc_dir_name": "Pkgconfig directory, relative to install directory",
        "lib_dir_name": "Lib directory, relative to install directory",
    },
)
