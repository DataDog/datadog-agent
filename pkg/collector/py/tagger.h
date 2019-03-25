// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

#ifndef TAGGER_HEADER
#define TAGGER_HEADER

#include <Python.h>

typedef enum {
  TC_FIRST = 0,
  LOW_CARD = TC_FIRST,
  ORCHESTRATOR_CARD,
  HIGH_CARD,
  TC_LAST = HIGH_CARD
} TaggerCardinality;


void inittagger();

#endif /* TAGGER_HEADER */
