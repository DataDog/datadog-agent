# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import List, Union, TYPE_CHECKING

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
    unset,
    UnsetType,
)


if TYPE_CHECKING:
    from datadog_api_client.v2.model.usage_time_series_object import UsageTimeSeriesObject
    from datadog_api_client.v2.model.hourly_usage_type import HourlyUsageType


class UsageAttributesObject(ModelNormal):
    @cached_property
    def openapi_types(_):
        from datadog_api_client.v2.model.usage_time_series_object import UsageTimeSeriesObject
        from datadog_api_client.v2.model.hourly_usage_type import HourlyUsageType

        return {
            "org_name": (str,),
            "product_family": (str,),
            "public_id": (str,),
            "region": (str,),
            "timeseries": ([UsageTimeSeriesObject],),
            "usage_type": (HourlyUsageType,),
        }

    attribute_map = {
        "org_name": "org_name",
        "product_family": "product_family",
        "public_id": "public_id",
        "region": "region",
        "timeseries": "timeseries",
        "usage_type": "usage_type",
    }

    def __init__(
        self_,
        org_name: Union[str, UnsetType] = unset,
        product_family: Union[str, UnsetType] = unset,
        public_id: Union[str, UnsetType] = unset,
        region: Union[str, UnsetType] = unset,
        timeseries: Union[List[UsageTimeSeriesObject], UnsetType] = unset,
        usage_type: Union[HourlyUsageType, UnsetType] = unset,
        **kwargs,
    ):
        """
        Usage attributes data.

        :param org_name: The organization name.
        :type org_name: str, optional

        :param product_family: The product for which usage is being reported.
        :type product_family: str, optional

        :param public_id: The organization public ID.
        :type public_id: str, optional

        :param region: The region of the Datadog instance that the organization belongs to.
        :type region: str, optional

        :param timeseries: List of usage data reported for each requested hour.
        :type timeseries: [UsageTimeSeriesObject], optional

        :param usage_type: Usage type that is being measured.
        :type usage_type: HourlyUsageType, optional
        """
        if org_name is not unset:
            kwargs["org_name"] = org_name
        if product_family is not unset:
            kwargs["product_family"] = product_family
        if public_id is not unset:
            kwargs["public_id"] = public_id
        if region is not unset:
            kwargs["region"] = region
        if timeseries is not unset:
            kwargs["timeseries"] = timeseries
        if usage_type is not unset:
            kwargs["usage_type"] = usage_type
        super().__init__(kwargs)
