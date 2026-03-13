"""DdPackagingInfo — common provider for the DD packaging decorator chain."""

DdPackagingInfo = provider(
    fields = {
        "installed_files": "list of PackageFilesInfo or PackageFilegroupInfo",
    },
)
