# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from enum import Enum

from deva.utils.models import BaseModel


class EnvironmentStage(str, Enum):
    ACTIVE = 'active'
    INACTIVE = 'inactive'
    STARTING = 'starting'
    STOPPING = 'stopping'


class EnvironmentStatus(BaseModel):
    stage: EnvironmentStage
    info: str = ''
