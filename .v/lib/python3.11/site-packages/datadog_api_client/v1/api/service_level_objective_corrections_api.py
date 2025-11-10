# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import Any, Dict, Union

from datadog_api_client.api_client import ApiClient, Endpoint as _Endpoint
from datadog_api_client.configuration import Configuration
from datadog_api_client.model_utils import (
    UnsetType,
    unset,
)
from datadog_api_client.v1.model.slo_correction_list_response import SLOCorrectionListResponse
from datadog_api_client.v1.model.slo_correction_response import SLOCorrectionResponse
from datadog_api_client.v1.model.slo_correction_create_request import SLOCorrectionCreateRequest
from datadog_api_client.v1.model.slo_correction_update_request import SLOCorrectionUpdateRequest


class ServiceLevelObjectiveCorrectionsApi:
    """
    SLO Status Corrections allow you to prevent specific time periods from negatively impacting
    your SLO’s status and error budget. You can use Status Corrections for various purposes, such
    as removing planned maintenance windows, non-business hours, or other time periods that do
    not correspond to genuine issues.
    """

    def __init__(self, api_client=None):
        if api_client is None:
            api_client = ApiClient(Configuration())
        self.api_client = api_client

        self._create_slo_correction_endpoint = _Endpoint(
            settings={
                "response_type": (SLOCorrectionResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/correction",
                "operation_id": "create_slo_correction",
                "http_method": "POST",
                "version": "v1",
            },
            params_map={
                "body": {
                    "required": True,
                    "openapi_types": (SLOCorrectionCreateRequest,),
                    "location": "body",
                },
            },
            headers_map={"accept": ["application/json"], "content_type": ["application/json"]},
            api_client=api_client,
        )

        self._delete_slo_correction_endpoint = _Endpoint(
            settings={
                "response_type": None,
                "auth": ["apiKeyAuth", "appKeyAuth"],
                "endpoint_path": "/api/v1/slo/correction/{slo_correction_id}",
                "operation_id": "delete_slo_correction",
                "http_method": "DELETE",
                "version": "v1",
            },
            params_map={
                "slo_correction_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_correction_id",
                    "location": "path",
                },
            },
            headers_map={
                "accept": ["*/*"],
            },
            api_client=api_client,
        )

        self._get_slo_correction_endpoint = _Endpoint(
            settings={
                "response_type": (SLOCorrectionResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth"],
                "endpoint_path": "/api/v1/slo/correction/{slo_correction_id}",
                "operation_id": "get_slo_correction",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "slo_correction_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_correction_id",
                    "location": "path",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._list_slo_correction_endpoint = _Endpoint(
            settings={
                "response_type": (SLOCorrectionListResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/correction",
                "operation_id": "list_slo_correction",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "offset": {
                    "openapi_types": (int,),
                    "attribute": "offset",
                    "location": "query",
                },
                "limit": {
                    "openapi_types": (int,),
                    "attribute": "limit",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._update_slo_correction_endpoint = _Endpoint(
            settings={
                "response_type": (SLOCorrectionResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth"],
                "endpoint_path": "/api/v1/slo/correction/{slo_correction_id}",
                "operation_id": "update_slo_correction",
                "http_method": "PATCH",
                "version": "v1",
            },
            params_map={
                "slo_correction_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_correction_id",
                    "location": "path",
                },
                "body": {
                    "required": True,
                    "openapi_types": (SLOCorrectionUpdateRequest,),
                    "location": "body",
                },
            },
            headers_map={"accept": ["application/json"], "content_type": ["application/json"]},
            api_client=api_client,
        )

    def create_slo_correction(
        self,
        body: SLOCorrectionCreateRequest,
    ) -> SLOCorrectionResponse:
        """Create an SLO correction.

        Create an SLO Correction.

        :param body: Create an SLO Correction
        :type body: SLOCorrectionCreateRequest
        :rtype: SLOCorrectionResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["body"] = body

        return self._create_slo_correction_endpoint.call_with_http_info(**kwargs)

    def delete_slo_correction(
        self,
        slo_correction_id: str,
    ) -> None:
        """Delete an SLO correction.

        Permanently delete the specified SLO correction object.

        :param slo_correction_id: The ID of the SLO correction object.
        :type slo_correction_id: str
        :rtype: None
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_correction_id"] = slo_correction_id

        return self._delete_slo_correction_endpoint.call_with_http_info(**kwargs)

    def get_slo_correction(
        self,
        slo_correction_id: str,
    ) -> SLOCorrectionResponse:
        """Get an SLO correction for an SLO.

        Get an SLO correction.

        :param slo_correction_id: The ID of the SLO correction object.
        :type slo_correction_id: str
        :rtype: SLOCorrectionResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_correction_id"] = slo_correction_id

        return self._get_slo_correction_endpoint.call_with_http_info(**kwargs)

    def list_slo_correction(
        self,
        *,
        offset: Union[int, UnsetType] = unset,
        limit: Union[int, UnsetType] = unset,
    ) -> SLOCorrectionListResponse:
        """Get all SLO corrections.

        Get all Service Level Objective corrections.

        :param offset: The specific offset to use as the beginning of the returned response.
        :type offset: int, optional
        :param limit: The number of SLO corrections to return in the response. Default is 25.
        :type limit: int, optional
        :rtype: SLOCorrectionListResponse
        """
        kwargs: Dict[str, Any] = {}
        if offset is not unset:
            kwargs["offset"] = offset

        if limit is not unset:
            kwargs["limit"] = limit

        return self._list_slo_correction_endpoint.call_with_http_info(**kwargs)

    def update_slo_correction(
        self,
        slo_correction_id: str,
        body: SLOCorrectionUpdateRequest,
    ) -> SLOCorrectionResponse:
        """Update an SLO correction.

        Update the specified SLO correction object.

        :param slo_correction_id: The ID of the SLO correction object.
        :type slo_correction_id: str
        :param body: The edited SLO correction object.
        :type body: SLOCorrectionUpdateRequest
        :rtype: SLOCorrectionResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_correction_id"] = slo_correction_id

        kwargs["body"] = body

        return self._update_slo_correction_endpoint.call_with_http_info(**kwargs)
