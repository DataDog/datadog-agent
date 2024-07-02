import json
from json.decoder import WHITESPACE
from typing import Any


class JSONWithCommentsDecoder(json.JSONDecoder):
    def decode(self, s: str, _w=WHITESPACE.match) -> Any:
        content_without_comments = '\n'.join(line for line in s.splitlines() if not line.lstrip().startswith('//'))
        return super().decode(content_without_comments)
