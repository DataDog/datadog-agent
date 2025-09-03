# (C) Datadog, Inc. 2018-present
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)
from __future__ import unicode_literals

import copy
import re
import socket
import time
from datetime import datetime, timezone
from urllib.parse import urlparse

import requests
from cryptography import x509
from requests import Response  # noqa: F401

from datadog_checks.base import AgentCheck, ensure_unicode, is_affirmative

from .config import DEFAULT_EXPECTED_CODE, from_instance
from .utils import get_ca_certs_path

DEFAULT_EXPIRE_DAYS_WARNING = 14
DEFAULT_EXPIRE_DAYS_CRITICAL = 7
DEFAULT_EXPIRE_WARNING = DEFAULT_EXPIRE_DAYS_WARNING * 24 * 3600
DEFAULT_EXPIRE_CRITICAL = DEFAULT_EXPIRE_DAYS_CRITICAL * 24 * 3600
MESSAGE_LENGTH = 2500  # https://docs.datadoghq.com/api/v1/service-checks/

DATA_METHODS = ["POST", "PUT", "DELETE", "PATCH", "OPTIONS"]


class HTTPCheck(AgentCheck):
    SOURCE_TYPE_NAME = "system"
    SC_STATUS = "http.can_connect"
    SC_SSL_CERT = "http.ssl_cert"
    HA_SUPPORTED = True

    DEFAULT_HTTP_CONFIG_REMAPPER = {
        "client_cert": {"name": "tls_cert"},
        "client_key": {"name": "tls_private_key"},
        "disable_ssl_validation": {
            "name": "tls_verify",
            "invert": True,
            "default": True,
        },
        "ignore_ssl_warning": {"name": "tls_ignore_warning"},
        "ca_certs": {"name": "tls_ca_cert"},
    }

    TLS_CONFIG_REMAPPER = {
        "check_hostname": {"name": "tls_validate_hostname"},
    }

    def __init__(self, name, init_config, instances):
        super(HTTPCheck, self).__init__(name, init_config, instances)

        self.HTTP_CONFIG_REMAPPER = copy.deepcopy(self.DEFAULT_HTTP_CONFIG_REMAPPER)

        self.ca_certs = init_config.get("ca_certs")
        if not self.ca_certs:
            self.ca_certs = get_ca_certs_path()

        if not self.instance.get("include_default_headers", True) and "headers" not in self.instance:
            headers = self.http.options["headers"]
            headers.clear()
            headers.update(self.instance.get("extra_headers", {}))

        if is_affirmative(self.instance.get('use_cert_from_response', False)):
            self.HTTP_CONFIG_REMAPPER['disable_ssl_validation']['default'] = False

    def check(self, instance):
        (
            addr,
            client_cert,
            client_key,
            method,
            data,
            http_response_status_code,
            include_content,
            headers,
            response_time,
            content_match,
            reverse_content_match,
            tags,
            ssl_expire,
            instance_ca_certs,
            check_hostname,
            stream,
            use_cert_from_response,
        ) = from_instance(instance, self.ca_certs)
        timeout = self.http.options["timeout"][0]
        start = time.time()

        def send_status_up(log_msg):
            # TODO: A6 log needs bytes and cannot handle unicode
            self.log.debug(log_msg)
            service_checks.append((self.SC_STATUS, AgentCheck.OK, "UP"))

        def send_status_down(loginfo, down_msg):
            # TODO: A6 log needs bytes and cannot handle unicode
            self.log.info(loginfo)
            down_msg = self._include_content(include_content, down_msg, r.text)
            service_checks.append((self.SC_STATUS, AgentCheck.CRITICAL, down_msg))

        # Store tags in a temporary list so that we don't modify the global tags data structure
        tags_list = list(tags)
        tags_list.append("url:{}".format(addr))
        instance_name = self.normalize_tag(instance["name"])
        tags_list.append("instance:{}".format(instance_name))
        service_checks = []
        service_checks_tags = self._get_service_checks_tags(instance)
        r = None  # type: Response
        peer_cert = None  # type: bytes | None
        try:
            parsed_uri = urlparse(addr)
            self.log.debug("Connecting to %s", addr)
            self.http.session.trust_env = False

            # Add 'Content-Type' for non GET requests when they have not been specified in custom headers
            if method.upper() in DATA_METHODS and not headers.get("Content-Type"):
                self.http.options["headers"]["Content-Type"] = "application/x-www-form-urlencoded"

            http_method = method.lower()
            if http_method == "options":
                http_method = "options_method"

            r = getattr(self.http, http_method)(
                addr,
                persist=True,
                stream=stream,
                json=data if method.upper() in DATA_METHODS and isinstance(data, dict) else None,
                data=data if method.upper() in DATA_METHODS and isinstance(data, str) else None,
            )
        except (
            socket.timeout,
            requests.exceptions.ConnectionError,
            requests.exceptions.Timeout,
        ) as e:
            length = int((time.time() - start) * 1000)
            self.log.info("%s is DOWN, error: %s. Connection failed after %s ms", addr, e, length)
            service_checks.append(
                (
                    self.SC_STATUS,
                    AgentCheck.CRITICAL,
                    "{}. Connection failed after {} ms".format(str(e), length),
                )
            )

        except socket.error as e:
            length = int((time.time() - start) * 1000)
            self.log.info(
                "%s is DOWN, error: %s. Connection failed after %s ms",
                addr,
                repr(e),
                length,
            )
            service_checks.append(
                (
                    self.SC_STATUS,
                    AgentCheck.CRITICAL,
                    "Socket error: {}. Connection failed after {} ms".format(repr(e), length),
                )
            )
        except IOError as e:  # Py2 throws IOError on invalid cert path while py3 throws a socket.error
            length = int((time.time() - start) * 1000)
            self.log.info(
                "Host %s could not be reached: %s. Connection failed after %s ms",
                addr,
                repr(e),
                length,
            )
            service_checks.append(
                (
                    self.SC_STATUS,
                    AgentCheck.CRITICAL,
                    "Socket error: {}. Connection failed after {} ms".format(repr(e), length),
                )
            )
        except Exception as e:
            length = int((time.time() - start) * 1000)
            self.log.error("Unhandled exception %s. Connection failed after %s ms", e, length)
            raise

        else:
            if use_cert_from_response:
                peer_cert = r.raw.connection.sock.getpeercert(binary_form=True)

            # Only add the URL tag if it's not already present
            if not any(filter(re.compile("^url:").match, tags_list)):
                tags_list.append("url:{}".format(addr))

            # Only report this metric if the site is not down
            if response_time and not service_checks:
                self.gauge(
                    "network.http.response_time",
                    r.elapsed.total_seconds(),
                    tags=tags_list,
                )

            # Check HTTP response status code
            if not (service_checks or re.match(http_response_status_code, str(r.status_code))):
                if http_response_status_code == DEFAULT_EXPECTED_CODE:
                    expected_code = "1xx or 2xx or 3xx"
                else:
                    expected_code = http_response_status_code

                message = "Incorrect HTTP return code for url {}. Expected {}, got {}.".format(
                    addr, expected_code, str(r.status_code)
                )
                message = self._include_content(include_content, message, r.text)

                self.log.info(message)

                service_checks.append((self.SC_STATUS, AgentCheck.CRITICAL, message))

            if not service_checks:
                # Host is UP
                # Check content matching is set
                if content_match:
                    if re.search(content_match, r.text, re.UNICODE):
                        if reverse_content_match:
                            send_status_down(
                                "{} is found in return content with the reverse_content_match option".format(
                                    ensure_unicode(content_match)
                                ),
                                'Content "{}" found in response with the reverse_content_match'.format(
                                    ensure_unicode(content_match)
                                ),
                            )
                        else:
                            send_status_up("{} is found in return content".format(ensure_unicode(content_match)))

                    else:
                        if reverse_content_match:
                            send_status_up(
                                "{} is not found in return content with the reverse_content_match option".format(
                                    ensure_unicode(content_match)
                                )
                            )
                        else:
                            send_status_down(
                                "{} is not found in return content".format(ensure_unicode(content_match)),
                                'Content "{}" not found in response.'.format(ensure_unicode(content_match)),
                            )

                else:
                    send_status_up("{} is UP".format(addr))
        finally:
            if r is not None:
                r.close()
            # resets the wrapper Session object
            self.http._session.close()
            self.http._session = None

        # Report status metrics as well
        if service_checks:
            can_status = 1 if service_checks[0][1] == AgentCheck.OK else 0
            self.gauge("network.http.can_connect", can_status, tags=tags_list)

            # cant_connect is useful for top lists
            cant_status = 0 if service_checks[0][1] == AgentCheck.OK else 1
            self.gauge("network.http.cant_connect", cant_status, tags=tags_list)

        if ssl_expire and parsed_uri.scheme == "https":
            if peer_cert is None:
                status, days_left, seconds_left, msg = self.check_cert_expiration(instance, timeout, instance_ca_certs)
            else:
                status, days_left, seconds_left, msg = self._inspect_cert(peer_cert, instance)

            tags_list = list(tags)
            tags_list.append("url:{}".format(addr))
            tags_list.append("instance:{}".format(instance_name))
            self.gauge("http.ssl.days_left", days_left, tags=tags_list)
            self.gauge("http.ssl.seconds_left", seconds_left, tags=tags_list)

            service_checks.append((self.SC_SSL_CERT, status, msg))

        for status in service_checks:
            sc_name, status, msg = status

            if status is AgentCheck.OK:
                msg = None
            self.report_as_service_check(sc_name, status, service_checks_tags, msg)

    def _get_service_checks_tags(self, instance):
        instance_name = self.normalize_tag(instance["name"])
        url = instance.get("url", None)
        if url is not None:
            url = ensure_unicode(url)
        tags = instance.get("tags", [])
        tags.append("instance:{}".format(instance_name))

        # Only add the URL tag if it's not already present
        if not any(filter(re.compile("^url:").match, tags)):
            tags.append("url:{}".format(url))
        return tags

    def report_as_service_check(self, sc_name, status, tags, msg=None):
        if sc_name == self.SC_STATUS:
            # format the HTTP response body into the event
            if isinstance(msg, tuple):
                code, reason, content = msg

                # truncate and html-escape content
                if len(content) > 200:
                    content = content[:197] + "..."

                msg = "{} {}\n\n{}".format(code, reason, content)
                msg = msg.rstrip()

        self.service_check(sc_name, status, tags=tags, message=msg)

    def check_cert_expiration(self, instance, timeout, instance_ca_certs):
        try:
            peer_cert = self._fetch_cert(instance, timeout, instance_ca_certs)
        except Exception as e:
            msg = repr(e)
            if any(word in msg for word in ['expired', 'expiration']):
                self.log.debug('error: %s. Cert might be expired.', e)
                return AgentCheck.CRITICAL, 0, 0, msg
            else:
                if 'Hostname mismatch' in msg or "doesn't match" in msg:
                    self.log.debug(
                        'The hostname on the SSL certificate does not match the given host: %s',
                        e,
                    )
                else:
                    self.log.debug('Unable to connect to site to get cert expiration: %s', e)
                return AgentCheck.UNKNOWN, None, None, msg

        # To maintain backwards compatability, if we aren't validating tls/certs, do not process
        # the returned binary cert unless specifically configured to with tls_retrieve_non_validated_cert
        if (
            not is_affirmative(instance.get('tls_verify', True))
            and not is_affirmative(instance.get('tls_retrieve_non_validated_cert', False))
        ) or not peer_cert:
            return AgentCheck.UNKNOWN, None, None, 'Empty or no certificate found.'
        else:
            return self._inspect_cert(peer_cert, instance)

    def _inspect_cert(self, binary_cert, instance):
        # thresholds expressed in seconds take precedence over those expressed in days
        seconds_warning = (
            int(instance.get("seconds_warning", 0))
            or int(instance.get("days_warning", 0)) * 24 * 3600
            or DEFAULT_EXPIRE_WARNING
        )
        seconds_critical = (
            int(instance.get("seconds_critical", 0))
            or int(instance.get("days_critical", 0)) * 24 * 3600
            or DEFAULT_EXPIRE_CRITICAL
        )

        try:
            cert = x509.load_der_x509_certificate(binary_cert)
            exp_date = cert.not_valid_after_utc
        except Exception as e:
            msg = repr(e)
            self.log.debug('Unable to parse the certificate to get expiration: %s', e)
            return AgentCheck.UNKNOWN, None, None, msg

        time_left = exp_date - datetime.now(timezone.utc)
        days_left = time_left.days
        seconds_left = time_left.total_seconds()

        self.log.debug("Exp_date: %s", exp_date)
        self.log.debug("seconds_left: %s", seconds_left)

        if seconds_left < seconds_critical:
            return (
                AgentCheck.CRITICAL,
                days_left,
                seconds_left,
                "This cert TTL is critical: only {} days before it expires".format(days_left),
            )

        elif seconds_left < seconds_warning:
            return (
                AgentCheck.WARNING,
                days_left,
                seconds_left,
                "This cert is almost expired, only {} days left".format(days_left),
            )

        else:
            return (
                AgentCheck.OK,
                days_left,
                seconds_left,
                "Days left: {}".format(days_left),
            )

    def _fetch_cert(self, instance, timeout, instance_ca_certs):
        url = instance.get('url')

        o = urlparse(url)
        host = o.hostname
        server_name = instance.get('ssl_server_name', o.hostname)
        port = o.port or 443

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(float(timeout))
        sock.connect((host, port))

        context = self.get_tls_context()
        context.load_verify_locations(instance_ca_certs)

        ssl_sock = context.wrap_socket(sock, server_hostname=server_name)
        return ssl_sock.getpeercert(binary_form=True)

    @staticmethod
    def _include_content(include_content, message, content):
        if include_content:
            message += "\nContent: {}".format(content[:MESSAGE_LENGTH])
            message = message[:MESSAGE_LENGTH]
        return message
