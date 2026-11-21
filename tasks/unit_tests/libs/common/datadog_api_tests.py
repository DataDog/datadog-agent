import gc
import json
import unittest
import weakref
from datetime import datetime, timedelta, timezone
from unittest.mock import patch

from datadog_api_client import ApiClient
from datadog_api_client.exceptions import ApiException, ServiceException
from datadog_api_client.model_utils import model_to_dict, validate_and_convert_types
from datadog_api_client.v2.model.ci_app_response_links import CIAppResponseLinks
from datadog_api_client.v2.model.ci_app_response_metadata_with_pagination import CIAppResponseMetadataWithPagination
from datadog_api_client.v2.model.ci_app_response_page import CIAppResponsePage
from datadog_api_client.v2.model.ci_app_sort import CIAppSort
from datadog_api_client.v2.model.ci_app_test_events_response import CIAppTestEventsResponse
from urllib3.exceptions import MaxRetryError, ProtocolError

from tasks.libs.common.datadog_api import iter_ci_test_events


def _event(identifier):
    return {"id": identifier, "type": "citest", "attributes": {"attributes": {"test": {"name": identifier}}}}


def _response(raw):
    # Convert model fixtures without a client; the cache regression uses real deserialize.
    return validate_and_convert_types(raw, (CIAppTestEventsResponse,), ["received_data"], True, True)


class TestIterCiTestEvents(unittest.TestCase):
    def setUp(self):
        self.start = datetime(2026, 10, 1, tzinfo=timezone.utc)
        self.end = datetime(2026, 10, 4, tzinfo=timezone.utc)
        client_patch = patch("datadog_api_client.ApiClient")
        self.client_class = client_patch.start()
        self.addCleanup(client_patch.stop)
        self.client_context = self.client_class.return_value
        api_patch = patch("datadog_api_client.v2.api.ci_visibility_tests_api.CIVisibilityTestsApi")
        self.api_class = api_patch.start()
        self.addCleanup(api_patch.stop)
        self.search = self.api_class.return_value.search_ci_app_test_events

    def test_pages_and_batches_share_window_and_preserve_retries_with_separate_clients(self):
        self.search.side_effect = [
            _response({"data": [_event("first")], "meta": {"page": {"after": "cursor"}}}),
            _response({"data": [_event("second")], "meta": {"status": "done"}}),
            _response({"data": [_event("third")], "meta": {"page": {"after": "cursor"}}}),
            _response({"data": []}),
        ]
        events = list(iter_ci_test_events(iter(["query-one", "query-two"]), self.start, self.end))
        self.assertEqual([event.id for event in events], ["first", "second", "third"])
        self.assertEqual(self.client_class.call_count, 4)
        for client_call in self.client_class.call_args_list:
            configuration = client_call.args[0]
            self.assertTrue(configuration.enable_retry)
            self.assertEqual(configuration.max_retries, 3)
            self.assertEqual(configuration.retry_backoff_factor, 2)
        self.assertEqual(self.api_class.call_count, 4)
        for api_call in self.api_class.call_args_list:
            self.assertEqual(api_call.args, (self.client_context.__enter__.return_value,))
        self.assertEqual(self.client_context.__exit__.call_count, 4)
        bodies = [call.kwargs["body"] for call in self.search.call_args_list]
        self.assertEqual([body.filter.query for body in bodies], ["query-one", "query-one", "query-two", "query-two"])
        self.assertEqual([body.page.get("cursor") for body in bodies], [None, "cursor", None, "cursor"])
        for body in bodies:
            self.assertEqual(body.filter._from, "2026-10-01T00:00:00+00:00")
            self.assertEqual(body.filter.to, "2026-10-04T00:00:00+00:00")
            self.assertEqual(body.sort, CIAppSort.TIMESTAMP_DESCENDING)
            self.assertEqual(body.page.limit, 1000)

    def test_optional_terminal_metadata_and_empty_jobs(self):
        terminal_forms = [
            {},
            {"meta": None},
            {"meta": {}},
            {"meta": {"page": None}},
            {"meta": {"page": {}}},
            {"meta": {"page": {"after": None}}},
            {"meta": {"page": {"after": ""}}},
            {"meta": {"status": "done", "warnings": []}},
            {"meta": {"status": None, "warnings": None}},
            {"links": None},
            {"links": {}},
            {"links": {"next": None}},
            {"links": {"next": ""}},
        ]
        for form in terminal_forms:
            for events in ([], [_event("one")]):
                with self.subTest(form=form, count=len(events)):
                    self.search.reset_mock()
                    self.search.return_value = _response({"data": events, **form})
                    result = list(iter_ci_test_events(["query"], self.start, self.end))
                    self.assertEqual([event.id for event in result], [event["id"] for event in events])
                    self.search.assert_called_once()

    def test_full_terminal_page_does_not_require_cursor(self):
        self.search.return_value = _response({"data": [_event(str(index)) for index in range(1000)]})
        result = list(iter_ci_test_events(["query"], self.start, self.end))
        self.assertEqual([event.id for event in result], [str(index) for index in range(1000)])
        self.search.assert_called_once()

    def test_short_and_empty_pages_continue_with_fresh_cursor(self):
        self.search.side_effect = [
            _response({"data": [_event("one")], "meta": {"page": {"after": "a"}}}),
            _response({"data": [], "meta": {"page": {"after": "b"}}}),
            _response({"data": [_event("two")]}),
        ]
        self.assertEqual([event.id for event in iter_ci_test_events(["query"], self.start, self.end)], ["one", "two"])
        self.assertEqual(self.search.call_count, 3)

    def test_full_page_continues_with_cursor(self):
        self.search.side_effect = [
            _response({"data": [_event(str(index)) for index in range(1000)], "meta": {"page": {"after": "a"}}}),
            _response({"data": [_event("last")]}),
        ]
        result = list(iter_ci_test_events(["query"], self.start, self.end))
        self.assertEqual([event.id for event in result], [str(index) for index in range(1000)] + ["last"])
        self.assertEqual(self.search.call_count, 2)

    def test_next_link_and_cursor_allow_continuation(self):
        self.search.side_effect = [
            _response(
                {
                    "data": [_event("one")],
                    "links": {"next": "https://example.invalid/events?page[cursor]=a"},
                    "meta": {"page": {"after": "a"}},
                }
            ),
            _response({"data": []}),
        ]
        self.assertEqual([event.id for event in iter_ci_test_events(["query"], self.start, self.end)], ["one"])
        self.assertEqual(self.search.call_count, 2)

    def test_next_link_without_cursor_rejects_page_before_yield(self):
        for meta in (None, {}, {"page": {}}, {"page": {"after": ""}}, {"page": {"after": None}}):
            with self.subTest(meta=meta):
                self.search.return_value = _response(
                    {"data": [_event("partial")], "meta": meta, "links": {"next": "https://example.invalid/next"}}
                )
                with self.assertRaisesRegex(ValueError, "next link without a continuation cursor"):
                    next(iter_ci_test_events(["query"], self.start, self.end))

    def test_repeated_cursor_rejects_page_before_yield(self):
        self.search.side_effect = [
            _response({"data": [_event("one")], "meta": {"page": {"after": "a"}}}),
            _response({"data": [_event("partial")], "meta": {"page": {"after": "a"}}}),
        ]
        events = iter_ci_test_events(["query"], self.start, self.end)
        self.assertEqual(next(events).id, "one")
        with self.assertRaisesRegex(ValueError, "repeated a continuation cursor"):
            next(events)
        self.assertEqual(self.search.call_count, 2)
        self.assertEqual(self.client_context.__exit__.call_count, 2)

    def test_cursor_cycle_is_rejected(self):
        self.search.side_effect = [
            _response({"data": [], "meta": {"page": {"after": cursor}}}) for cursor in ("a", "b", "a")
        ]
        with self.assertRaisesRegex(ValueError, "repeated a continuation cursor"):
            list(iter_ci_test_events(["query"], self.start, self.end))
        self.assertEqual(self.search.call_count, 3)

    def test_timeout_and_unknown_status_reject_page_before_yield(self):
        for status in ("timeout", "unexpected-status"):
            with self.subTest(status=status):
                self.search.return_value = _response({"data": [_event("partial")], "meta": {"status": status}})
                with self.assertRaisesRegex(ValueError, "did not complete"):
                    next(iter_ci_test_events(["query"], self.start, self.end))

    def test_warnings_reject_page_before_yield(self):
        self.search.return_value = _response(
            {"data": [_event("partial")], "meta": {"status": "done", "warnings": [{"code": "partial"}]}}
        )
        with self.assertRaisesRegex(ValueError, "warnings; results may be incomplete"):
            next(iter_ci_test_events(["query"], self.start, self.end))
        self.client_context.__exit__.assert_called_once()

    def test_missing_or_malformed_data_is_not_an_empty_result(self):
        responses = [CIAppTestEventsResponse()]
        responses.extend(
            CIAppTestEventsResponse(data=data, _check_type=False)
            for data in (None, {}, "", (), [None], ["event"], [{"id": "raw-dict"}])
        )
        for response in responses:
            with self.subTest(response=response):
                self.search.return_value = response
                with self.assertRaisesRegex(ValueError, "missing or invalid data"):
                    list(iter_ci_test_events(["query"], self.start, self.end))

    def test_malformed_response_is_rejected(self):
        for response in (None, {}, []):
            with self.subTest(response=response):
                self.search.return_value = response
                with self.assertRaisesRegex(ValueError, "invalid response"):
                    list(iter_ci_test_events(["query"], self.start, self.end))

    def test_malformed_metadata_is_rejected(self):
        malformed = [
            ({"meta": ""}, "invalid metadata"),
            ({"meta": CIAppResponseMetadataWithPagination(page="", _check_type=False)}, "invalid pagination"),
            (
                {"meta": CIAppResponseMetadataWithPagination(page=CIAppResponsePage(after=123, _check_type=False))},
                "invalid cursor",
            ),
            ({"meta": CIAppResponseMetadataWithPagination(warnings="", _check_type=False)}, "warnings"),
            ({"links": ""}, "invalid links"),
            ({"links": CIAppResponseLinks(next=123, _check_type=False)}, "invalid next link"),
        ]
        for fields, error in malformed:
            with self.subTest(fields=fields):
                self.search.return_value = CIAppTestEventsResponse(data=[], _check_type=False, **fields)
                with self.assertRaisesRegex(ValueError, error):
                    list(iter_ci_test_events(["query"], self.start, self.end))

    def test_exhausted_rate_limit_and_server_errors_propagate(self):
        for error in (ApiException(status=429, reason="exhausted"), ServiceException(status=503, reason="exhausted")):
            with self.subTest(error=type(error).__name__):
                self.search.side_effect = error
                with self.assertRaises(type(error)) as caught:
                    list(iter_ci_test_events(["query"], self.start, self.end))
                self.assertIs(caught.exception, error)

    def test_transport_failures_propagate(self):
        for error in (MaxRetryError(None, "synthetic", "exhausted"), ProtocolError("connection lost")):
            with self.subTest(error=type(error).__name__):
                self.search.side_effect = error
                with self.assertRaises(type(error)) as caught:
                    list(iter_ci_test_events(["query"], self.start, self.end))
                self.assertIs(caught.exception, error)

    def test_later_page_failure_propagates_and_closes_client(self):
        error = ApiException(status=429, reason="exhausted")
        self.search.side_effect = [
            _response({"data": [_event("one")], "meta": {"page": {"after": "a"}}}),
            error,
        ]
        events = iter_ci_test_events(["query"], self.start, self.end)
        self.assertEqual(next(events).id, "one")
        with self.assertRaises(ApiException) as caught:
            next(events)
        self.assertIs(caught.exception, error)
        self.assertEqual(self.client_context.__exit__.call_count, 2)

    def test_early_close_closes_client(self):
        self.search.return_value = _response({"data": [_event("one"), _event("two")], "meta": {"page": {"after": "a"}}})
        events = iter_ci_test_events(["query"], self.start, self.end)
        self.assertEqual(next(events).id, "one")
        events.close()
        self.search.assert_called_once()
        self.client_context.__exit__.assert_called_once()

    def test_fetches_next_page_only_when_consumed(self):
        self.search.side_effect = [
            _response({"data": [_event("one"), _event("two")], "meta": {"page": {"after": "a"}}}),
            _response({"data": [_event("three")]}),
        ]
        events = iter_ci_test_events(["query"], self.start, self.end)
        self.search.assert_not_called()
        self.assertEqual(next(events).id, "one")
        self.assertEqual(next(events).id, "two")
        self.search.assert_called_once()
        self.assertEqual(next(events).id, "three")
        self.assertEqual(self.search.call_count, 2)
        self.assertEqual(list(events), [])

    def test_empty_queries_make_no_request(self):
        self.assertEqual(list(iter_ci_test_events([], self.start, self.end)), [])
        self.search.assert_not_called()
        self.client_class.assert_not_called()

    def test_invalid_queries_make_no_request(self):
        for query in ("", None, 42):
            with self.subTest(query=query):
                with self.assertRaisesRegex(ValueError, "nonempty strings"):
                    list(iter_ci_test_events([query], self.start, self.end))
        self.search.assert_not_called()
        self.client_class.assert_not_called()

    def test_oversized_query_rejected_before_request(self):
        # Non-ASCII text is escaped by the SDK's JSON serializer.
        for query in ("q" * 16384, "\u00e9" * 3000):
            with self.subTest(character=query[0]):
                with self.assertRaisesRegex(ValueError, "local 16384-byte limit"):
                    list(iter_ci_test_events([query], self.start, self.end))
        self.search.assert_not_called()
        self.client_class.assert_not_called()

    def test_request_at_local_size_limit_is_accepted(self):
        empty_body = {
            "filter": {"from": self.start.isoformat(), "query": "", "to": self.end.isoformat()},
            "page": {"limit": 1000},
            "sort": "-timestamp",
        }
        query = "q" * (16384 - len(json.dumps(empty_body).encode("utf-8")))
        self.search.return_value = _response({"data": []})
        self.assertEqual(list(iter_ci_test_events([query], self.start, self.end)), [])
        sent_body = model_to_dict(self.search.call_args.kwargs["body"], serialize=True)
        self.assertEqual(len(json.dumps(sent_body).encode("utf-8")), 16384)

    def test_oversized_cursor_rejected_before_following_request(self):
        self.search.return_value = _response({"data": [_event("one")], "meta": {"page": {"after": "c" * 16384}}})
        events = iter_ci_test_events(["query"], self.start, self.end)
        self.assertEqual(next(events).id, "one")
        with self.assertRaisesRegex(ValueError, "local 16384-byte limit"):
            next(events)
        self.search.assert_called_once()
        self.client_context.__exit__.assert_called_once()

    def test_timezone_bounds_are_normalized_without_moving_window(self):
        offset = timezone(timedelta(hours=3))
        start = datetime(2026, 10, 1, 3, tzinfo=offset)
        end = datetime(2026, 10, 4, 3, tzinfo=offset)
        self.search.return_value = _response({"data": []})
        list(iter_ci_test_events(["query"], start, end))
        body = self.search.call_args.kwargs["body"]
        self.assertEqual(body.filter._from, "2026-10-01T00:00:00+00:00")
        self.assertEqual(body.filter.to, "2026-10-04T00:00:00+00:00")

    def test_invalid_window_rejected_before_client_creation(self):
        for start, end in (
            (self.start.replace(tzinfo=None), self.end),
            (self.start, self.end.replace(tzinfo=None)),
            (self.end, self.start),
        ):
            with self.subTest(start=start, end=end):
                with self.assertRaisesRegex(ValueError, "timezone-aware and ordered"):
                    list(iter_ci_test_events(["query"], start, end))
        self.client_class.assert_not_called()

    def test_equal_window_bounds_are_preserved(self):
        self.search.return_value = _response({"data": []})
        list(iter_ci_test_events(["query"], self.start, self.start))
        body = self.search.call_args.kwargs["body"]
        self.assertEqual(body.filter._from, body.filter.to)


class TestCiTestEventsClientLifetime(unittest.TestCase):
    def test_actual_sdk_clients_and_cached_pages_are_released_during_iteration(self):
        pages = iter(
            [
                {"data": [_event("one")], "meta": {"page": {"after": "a"}}},
                {"data": [_event("two")]},
            ]
        )
        client_refs = []

        def search(api, *, body):
            client = api.api_client
            self.assertIsInstance(client, ApiClient)
            client_refs.append(weakref.ref(client))
            response = client.deserialize(json.dumps(next(pages)), (CIAppTestEventsResponse,), True)
            # SDK 2.46 retains decoded responses here; older versions have no cache.
            cache = getattr(client, "_validation_cache", None)
            if cache is not None:
                self.assertTrue(cache)
            return response

        with (
            patch(
                "datadog_api_client.v2.api.ci_visibility_tests_api.CIVisibilityTestsApi.search_ci_app_test_events",
                new=search,
            ),
            patch("datadog_api_client.rest.RESTClientObject.request", side_effect=AssertionError("Unexpected network")),
        ):
            start = datetime(2026, 10, 1, tzinfo=timezone.utc)
            end = datetime(2026, 10, 4, tzinfo=timezone.utc)
            events = iter_ci_test_events(["query"], start, end)
            first = next(events)
            self.assertEqual(first.id, "one")
            first_ref = weakref.ref(first)
            del first
            gc.collect()
            self.assertEqual(len(client_refs), 1)
            self.assertIsNone(client_refs[0]())

            self.assertEqual(next(events).id, "two")
            gc.collect()
            self.assertEqual(len(client_refs), 2)
            self.assertTrue(all(reference() is None for reference in client_refs))
            self.assertIsNone(first_ref())
            self.assertEqual(list(events), [])
