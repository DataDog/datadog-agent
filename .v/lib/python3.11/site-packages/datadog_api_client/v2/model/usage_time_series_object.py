# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import Union

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
    datetime,
    none_type,
    unset,
    UnsetType,
)


class UsageTimeSeriesObject(ModelNormal):
    @cached_property
    def openapi_types(_):
        return {
            "timestamp": (datetime,),
            "value": (int, none_type),
        }

    attribute_map = {
        "timestamp": "timestamp",
        "value": "value",
    }

    def __init__(
        self_, timestamp: Union[datetime, UnsetType] = unset, value: Union[int, none_type, UnsetType] = unset, **kwargs
    ):
        """
        Usage timeseries data.

        :param timestamp: Datetime in ISO-8601 format, UTC. The hour for the usage.
        :type timestamp: datetime, optional

        :param value: Contains the number measured for the given usage_type during the hour.
        :type value: int, none_type, optional
        """
        if timestamp is not unset:
            kwargs["timestamp"] = timestamp
        if value is not unset:
            kwargs["value"] = value
        super().__init__(kwargs)
