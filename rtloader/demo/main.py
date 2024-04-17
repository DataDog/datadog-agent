# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-present Datadog, Inc.
from __future__ import print_function  # fmt: off


import aggregator
import tagger

if __name__ == "__main__":
    aggregator.submit_metric(None, 'id', aggregator.GAUGE, 'name', -99.0, ['foo', 'bar'], 'myhost', False)
    print(f"tags returned by tagger: {tagger.get_tags('21', True)}\n")
