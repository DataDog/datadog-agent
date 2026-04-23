from __future__ import annotations

import json
import os
import urllib.error
import urllib.request

from tools.logs_agent_ai.constants import (
    AI_API_KEY_ENV,
    AI_BASE_URL_ENV,
    AI_ORG_ID_ENV,
    AI_SOURCE_ENV,
    DEFAULT_AI_BASE_URL,
)


class LLMError(RuntimeError):
    pass


class LLMClient:
    def __init__(self, api_key: str | None = None, base_url: str | None = None):
        self.api_key = api_key or os.environ.get(AI_API_KEY_ENV)
        configured_base_url = base_url or os.environ.get(AI_BASE_URL_ENV) or DEFAULT_AI_BASE_URL
        self.base_url = _normalize_base_url(configured_base_url)
        self.source = os.environ.get(AI_SOURCE_ENV)
        self.org_id = os.environ.get(AI_ORG_ID_ENV)
        if not self.api_key:
            raise LLMError(f"Missing API key in {AI_API_KEY_ENV}")

    def generate_markdown(self, model: str, system_prompt: str, user_prompt: str) -> str:
        payload = self._post(
            {
                "model": model,
                "temperature": 0.1,
                "messages": [
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_prompt},
                ],
            }
        )
        return _extract_message_content(payload)

    def generate_json(self, model: str, system_prompt: str, user_prompt: str) -> dict[str, object]:
        raw = self.generate_markdown(model, system_prompt, user_prompt)
        return _load_json(raw)

    def _post(self, payload: dict[str, object]) -> dict[str, object]:
        body = json.dumps(payload).encode("utf-8")
        request = urllib.request.Request(
            url=f"{self.base_url}/chat/completions",
            data=body,
            headers=self._build_headers(),
            method="POST",
        )
        try:
            with urllib.request.urlopen(request) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            message = error.read().decode("utf-8", errors="replace")
            raise LLMError(f"LLM request failed: {error.code} {message}") from error
        except urllib.error.URLError as error:
            raise LLMError(f"LLM request failed: {error}") from error

    def _build_headers(self) -> dict[str, str]:
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }
        if self.source:
            headers["source"] = self.source
        if self.org_id:
            headers["org-id"] = self.org_id
        return headers


def _normalize_base_url(base_url: str) -> str:
    normalized = base_url.rstrip("/")
    if not normalized.endswith("/v1"):
        normalized = f"{normalized}/v1"
    return normalized


def _extract_message_content(payload: dict[str, object]) -> str:
    try:
        choices = payload["choices"]
        message = choices[0]["message"]
        content = message["content"]
    except (KeyError, IndexError, TypeError) as error:
        raise LLMError(f"Unexpected LLM response shape: {payload}") from error
    if not isinstance(content, str):
        raise LLMError(f"Unexpected LLM content: {content!r}")
    return content.strip()


def _load_json(raw: str) -> dict[str, object]:
    stripped = raw.strip()
    if stripped.startswith("```"):
        lines = stripped.splitlines()
        stripped = "\n".join(lines[1:-1]).strip()
    try:
        data = json.loads(stripped)
    except json.JSONDecodeError as error:
        raise LLMError(f"Model did not return valid JSON: {raw}") from error
    if not isinstance(data, dict):
        raise LLMError(f"Model JSON must be an object: {data!r}")
    return data
