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
from datadog_api_client.v1.model.slo_list_response import SLOListResponse
from datadog_api_client.v1.model.service_level_objective_request import ServiceLevelObjectiveRequest
from datadog_api_client.v1.model.slo_bulk_delete_response import SLOBulkDeleteResponse
from datadog_api_client.v1.model.slo_bulk_delete import SLOBulkDelete
from datadog_api_client.v1.model.check_can_delete_slo_response import CheckCanDeleteSLOResponse
from datadog_api_client.v1.model.search_slo_response import SearchSLOResponse
from datadog_api_client.v1.model.slo_delete_response import SLODeleteResponse
from datadog_api_client.v1.model.slo_response import SLOResponse
from datadog_api_client.v1.model.service_level_objective import ServiceLevelObjective
from datadog_api_client.v1.model.slo_correction_list_response import SLOCorrectionListResponse
from datadog_api_client.v1.model.slo_history_response import SLOHistoryResponse


class ServiceLevelObjectivesApi:
    """
    `Service Level Objectives <https://docs.datadoghq.com/monitors/service_level_objectives/#configuration>`_
    (or SLOs) are a key part of the site reliability engineering toolkit.
    SLOs provide a framework for defining clear targets around application performance,
    which ultimately help teams provide a consistent customer experience,
    balance feature development with platform stability,
    and improve communication with internal and external users.
    """

    def __init__(self, api_client=None):
        if api_client is None:
            api_client = ApiClient(Configuration())
        self.api_client = api_client

        self._check_can_delete_slo_endpoint = _Endpoint(
            settings={
                "response_type": (CheckCanDeleteSLOResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/can_delete",
                "operation_id": "check_can_delete_slo",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "ids": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "ids",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._create_slo_endpoint = _Endpoint(
            settings={
                "response_type": (SLOListResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo",
                "operation_id": "create_slo",
                "http_method": "POST",
                "version": "v1",
            },
            params_map={
                "body": {
                    "required": True,
                    "openapi_types": (ServiceLevelObjectiveRequest,),
                    "location": "body",
                },
            },
            headers_map={"accept": ["application/json"], "content_type": ["application/json"]},
            api_client=api_client,
        )

        self._delete_slo_endpoint = _Endpoint(
            settings={
                "response_type": (SLODeleteResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/{slo_id}",
                "operation_id": "delete_slo",
                "http_method": "DELETE",
                "version": "v1",
            },
            params_map={
                "slo_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_id",
                    "location": "path",
                },
                "force": {
                    "openapi_types": (str,),
                    "attribute": "force",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._delete_slo_timeframe_in_bulk_endpoint = _Endpoint(
            settings={
                "response_type": (SLOBulkDeleteResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/bulk_delete",
                "operation_id": "delete_slo_timeframe_in_bulk",
                "http_method": "POST",
                "version": "v1",
            },
            params_map={
                "body": {
                    "required": True,
                    "openapi_types": (SLOBulkDelete,),
                    "location": "body",
                },
            },
            headers_map={"accept": ["application/json"], "content_type": ["application/json"]},
            api_client=api_client,
        )

        self._get_slo_endpoint = _Endpoint(
            settings={
                "response_type": (SLOResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/{slo_id}",
                "operation_id": "get_slo",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "slo_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_id",
                    "location": "path",
                },
                "with_configured_alert_ids": {
                    "openapi_types": (bool,),
                    "attribute": "with_configured_alert_ids",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._get_slo_corrections_endpoint = _Endpoint(
            settings={
                "response_type": (SLOCorrectionListResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/{slo_id}/corrections",
                "operation_id": "get_slo_corrections",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "slo_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_id",
                    "location": "path",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._get_slo_history_endpoint = _Endpoint(
            settings={
                "response_type": (SLOHistoryResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/{slo_id}/history",
                "operation_id": "get_slo_history",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "slo_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_id",
                    "location": "path",
                },
                "from_ts": {
                    "required": True,
                    "openapi_types": (int,),
                    "attribute": "from_ts",
                    "location": "query",
                },
                "to_ts": {
                    "required": True,
                    "openapi_types": (int,),
                    "attribute": "to_ts",
                    "location": "query",
                },
                "target": {
                    "validation": {
                        "exclusive_maximum": 100,
                        "exclusive_minimum": 0,
                    },
                    "openapi_types": (float,),
                    "attribute": "target",
                    "location": "query",
                },
                "apply_correction": {
                    "openapi_types": (bool,),
                    "attribute": "apply_correction",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._list_slos_endpoint = _Endpoint(
            settings={
                "response_type": (SLOListResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo",
                "operation_id": "list_slos",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "ids": {
                    "openapi_types": (str,),
                    "attribute": "ids",
                    "location": "query",
                },
                "query": {
                    "openapi_types": (str,),
                    "attribute": "query",
                    "location": "query",
                },
                "tags_query": {
                    "openapi_types": (str,),
                    "attribute": "tags_query",
                    "location": "query",
                },
                "metrics_query": {
                    "openapi_types": (str,),
                    "attribute": "metrics_query",
                    "location": "query",
                },
                "limit": {
                    "openapi_types": (int,),
                    "attribute": "limit",
                    "location": "query",
                },
                "offset": {
                    "openapi_types": (int,),
                    "attribute": "offset",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._search_slo_endpoint = _Endpoint(
            settings={
                "response_type": (SearchSLOResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/search",
                "operation_id": "search_slo",
                "http_method": "GET",
                "version": "v1",
            },
            params_map={
                "query": {
                    "openapi_types": (str,),
                    "attribute": "query",
                    "location": "query",
                },
                "page_size": {
                    "openapi_types": (int,),
                    "attribute": "page[size]",
                    "location": "query",
                },
                "page_number": {
                    "openapi_types": (int,),
                    "attribute": "page[number]",
                    "location": "query",
                },
                "include_facets": {
                    "openapi_types": (bool,),
                    "attribute": "include_facets",
                    "location": "query",
                },
            },
            headers_map={
                "accept": ["application/json"],
            },
            api_client=api_client,
        )

        self._update_slo_endpoint = _Endpoint(
            settings={
                "response_type": (SLOListResponse,),
                "auth": ["apiKeyAuth", "appKeyAuth", "AuthZ"],
                "endpoint_path": "/api/v1/slo/{slo_id}",
                "operation_id": "update_slo",
                "http_method": "PUT",
                "version": "v1",
            },
            params_map={
                "slo_id": {
                    "required": True,
                    "openapi_types": (str,),
                    "attribute": "slo_id",
                    "location": "path",
                },
                "body": {
                    "required": True,
                    "openapi_types": (ServiceLevelObjective,),
                    "location": "body",
                },
            },
            headers_map={"accept": ["application/json"], "content_type": ["application/json"]},
            api_client=api_client,
        )

    def check_can_delete_slo(
        self,
        ids: str,
    ) -> CheckCanDeleteSLOResponse:
        """Check if SLOs can be safely deleted.

        Check if an SLO can be safely deleted. For example,
        assure an SLO can be deleted without disrupting a dashboard.

        :param ids: A comma separated list of the IDs of the service level objectives objects.
        :type ids: str
        :rtype: CheckCanDeleteSLOResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["ids"] = ids

        return self._check_can_delete_slo_endpoint.call_with_http_info(**kwargs)

    def create_slo(
        self,
        body: ServiceLevelObjectiveRequest,
    ) -> SLOListResponse:
        """Create an SLO object.

        Create a service level objective object.

        :param body: Service level objective request object.
        :type body: ServiceLevelObjectiveRequest
        :rtype: SLOListResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["body"] = body

        return self._create_slo_endpoint.call_with_http_info(**kwargs)

    def delete_slo(
        self,
        slo_id: str,
        *,
        force: Union[str, UnsetType] = unset,
    ) -> SLODeleteResponse:
        """Delete an SLO.

        Permanently delete the specified service level objective object.

        If an SLO is used in a dashboard, the ``DELETE /v1/slo/`` endpoint returns
        a 409 conflict error because the SLO is referenced in a dashboard.

        :param slo_id: The ID of the service level objective.
        :type slo_id: str
        :param force: Delete the monitor even if it's referenced by other resources (for example SLO, composite monitor).
        :type force: str, optional
        :rtype: SLODeleteResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_id"] = slo_id

        if force is not unset:
            kwargs["force"] = force

        return self._delete_slo_endpoint.call_with_http_info(**kwargs)

    def delete_slo_timeframe_in_bulk(
        self,
        body: SLOBulkDelete,
    ) -> SLOBulkDeleteResponse:
        """Bulk Delete SLO Timeframes.

        Delete (or partially delete) multiple service level objective objects.

        This endpoint facilitates deletion of one or more thresholds for one or more
        service level objective objects. If all thresholds are deleted, the service level
        objective object is deleted as well.

        :param body: Delete multiple service level objective objects request body.
        :type body: SLOBulkDelete
        :rtype: SLOBulkDeleteResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["body"] = body

        return self._delete_slo_timeframe_in_bulk_endpoint.call_with_http_info(**kwargs)

    def get_slo(
        self,
        slo_id: str,
        *,
        with_configured_alert_ids: Union[bool, UnsetType] = unset,
    ) -> SLOResponse:
        """Get an SLO's details.

        Get a service level objective object.

        :param slo_id: The ID of the service level objective object.
        :type slo_id: str
        :param with_configured_alert_ids: Get the IDs of SLO monitors that reference this SLO.
        :type with_configured_alert_ids: bool, optional
        :rtype: SLOResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_id"] = slo_id

        if with_configured_alert_ids is not unset:
            kwargs["with_configured_alert_ids"] = with_configured_alert_ids

        return self._get_slo_endpoint.call_with_http_info(**kwargs)

    def get_slo_corrections(
        self,
        slo_id: str,
    ) -> SLOCorrectionListResponse:
        """Get Corrections For an SLO.

        Get corrections applied to an SLO

        :param slo_id: The ID of the service level objective object.
        :type slo_id: str
        :rtype: SLOCorrectionListResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_id"] = slo_id

        return self._get_slo_corrections_endpoint.call_with_http_info(**kwargs)

    def get_slo_history(
        self,
        slo_id: str,
        from_ts: int,
        to_ts: int,
        *,
        target: Union[float, UnsetType] = unset,
        apply_correction: Union[bool, UnsetType] = unset,
    ) -> SLOHistoryResponse:
        """Get an SLO's history.

        Get a specific SLO’s history, regardless of its SLO type.

        The detailed history data is structured according to the source data type.
        For example, metric data is included for event SLOs that use
        the metric source, and monitor SLO types include the monitor transition history.

        **Note:** There are different response formats for event based and time based SLOs.
        Examples of both are shown.

        :param slo_id: The ID of the service level objective object.
        :type slo_id: str
        :param from_ts: The ``from`` timestamp for the query window in epoch seconds.
        :type from_ts: int
        :param to_ts: The ``to`` timestamp for the query window in epoch seconds.
        :type to_ts: int
        :param target: The SLO target. If ``target`` is passed in, the response will include the remaining error budget and a timeframe value of ``custom``.
        :type target: float, optional
        :param apply_correction: Defaults to ``true``. If any SLO corrections are applied and this parameter is set to ``false`` ,
            then the corrections will not be applied and the SLI values will not be affected.
        :type apply_correction: bool, optional
        :rtype: SLOHistoryResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_id"] = slo_id

        kwargs["from_ts"] = from_ts

        kwargs["to_ts"] = to_ts

        if target is not unset:
            kwargs["target"] = target

        if apply_correction is not unset:
            kwargs["apply_correction"] = apply_correction

        return self._get_slo_history_endpoint.call_with_http_info(**kwargs)

    def list_slos(
        self,
        *,
        ids: Union[str, UnsetType] = unset,
        query: Union[str, UnsetType] = unset,
        tags_query: Union[str, UnsetType] = unset,
        metrics_query: Union[str, UnsetType] = unset,
        limit: Union[int, UnsetType] = unset,
        offset: Union[int, UnsetType] = unset,
    ) -> SLOListResponse:
        """Get all SLOs.

        Get a list of service level objective objects for your organization.

        :param ids: A comma separated list of the IDs of the service level objectives objects.
        :type ids: str, optional
        :param query: The query string to filter results based on SLO names.
        :type query: str, optional
        :param tags_query: The query string to filter results based on a single SLO tag.
        :type tags_query: str, optional
        :param metrics_query: The query string to filter results based on SLO numerator and denominator.
        :type metrics_query: str, optional
        :param limit: The number of SLOs to return in the response.
        :type limit: int, optional
        :param offset: The specific offset to use as the beginning of the returned response.
        :type offset: int, optional
        :rtype: SLOListResponse
        """
        kwargs: Dict[str, Any] = {}
        if ids is not unset:
            kwargs["ids"] = ids

        if query is not unset:
            kwargs["query"] = query

        if tags_query is not unset:
            kwargs["tags_query"] = tags_query

        if metrics_query is not unset:
            kwargs["metrics_query"] = metrics_query

        if limit is not unset:
            kwargs["limit"] = limit

        if offset is not unset:
            kwargs["offset"] = offset

        return self._list_slos_endpoint.call_with_http_info(**kwargs)

    def search_slo(
        self,
        *,
        query: Union[str, UnsetType] = unset,
        page_size: Union[int, UnsetType] = unset,
        page_number: Union[int, UnsetType] = unset,
        include_facets: Union[bool, UnsetType] = unset,
    ) -> SearchSLOResponse:
        """Search for SLOs.

        Get a list of service level objective objects for your organization.

        :param query: The query string to filter results based on SLO names.
            Some examples of queries include ``service:<service-name>``
            and ``<slo-name>``.
        :type query: str, optional
        :param page_size: The number of files to return in the response ``[default=10]``.
        :type page_size: int, optional
        :param page_number: The identifier of the first page to return. This parameter is used for the pagination feature ``[default=0]``.
        :type page_number: int, optional
        :param include_facets: Whether or not to return facet information in the response ``[default=false]``.
        :type include_facets: bool, optional
        :rtype: SearchSLOResponse
        """
        kwargs: Dict[str, Any] = {}
        if query is not unset:
            kwargs["query"] = query

        if page_size is not unset:
            kwargs["page_size"] = page_size

        if page_number is not unset:
            kwargs["page_number"] = page_number

        if include_facets is not unset:
            kwargs["include_facets"] = include_facets

        return self._search_slo_endpoint.call_with_http_info(**kwargs)

    def update_slo(
        self,
        slo_id: str,
        body: ServiceLevelObjective,
    ) -> SLOListResponse:
        """Update an SLO.

        Update the specified service level objective object.

        :param slo_id: The ID of the service level objective object.
        :type slo_id: str
        :param body: The edited service level objective request object.
        :type body: ServiceLevelObjective
        :rtype: SLOListResponse
        """
        kwargs: Dict[str, Any] = {}
        kwargs["slo_id"] = slo_id

        kwargs["body"] = body

        return self._update_slo_endpoint.call_with_http_info(**kwargs)
