"""DdPackagingInfo — common provider for the DD packaging decorator chain."""

DdPackagingInfo = provider(
    fields = {
        "files": "depset of struct(file=File, prefix=str).",
    },
)
