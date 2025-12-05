from __future__ import annotations

from typing import Annotated

from msgspec import Meta, Struct


class CITrigger(Struct):
    changes: list[Annotated[str, Meta(min_length=1)]] = []

    def __post_init__(self) -> None:
        if self.changes:
            relative_paths = {}
            for i, path in enumerate(self.changes):
                relative_paths.setdefault(path, []).append(i)

            errors = []
            for path, indices in relative_paths.items():
                if len(indices) > 1:
                    msg = f"Duplicate path `{path}` at indices {', '.join(str(i) for i in indices)}"
                    errors.append(msg)

            if errors:
                raise ValueError("\n".join(errors))


class CIPipeline(Struct, tag_field="type"):
    pass


class CIStaticPipeline(CIPipeline, tag="static"):
    path: Annotated[str, Meta(min_length=1)]


class CIDynamicPipeline(CIPipeline, tag="dynamic"):
    command: Annotated[str, Meta(min_length=1)]


class CIUnit(Struct):
    id: Annotated[str, Meta(min_length=1)]
    name: Annotated[str, Meta(min_length=1)]
    pipeline: CIStaticPipeline | CIDynamicPipeline
    triggers: list[CITrigger] = []
