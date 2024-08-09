# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

import msgspec


class BaseModel(msgspec.Struct, forbid_unknown_fields=True):
    def validate(self):
        # https://github.com/jcrist/msgspec/issues/513
        # https://github.com/jcrist/msgspec/issues/89
        return msgspec.json.decode(msgspec.json.encode(self), type=type(self))
