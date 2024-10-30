// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

/*
#include <stdlib.h>
#include <string.h>

int arraylen(char **array, int max_len) {
	int i;
	for (i = 0; array[i]; i++){
		if (i == max_len) {
			return -1;
		}
	}
	return i;
}

*/
import "C"

func testTags(t *testing.T) {
	sender := mocksender.NewMockSender(checkid.ID("testID"))
	logReceiver := optional.NewNoneOption[integrations.Component]()
	tagger := taggerimpl.SetupFakeTagger(t)
	tagger.SetTags(types.NewEntityID(types.ContainerID, "test"), "foo", []string{"tag1", "tag2", "tag3"}, nil, nil, nil)
	release := scopeInitCheckContext(sender.GetSenderManager(), logReceiver, tagger)
	defer release()

	id := C.CString("container_id://test")
	defer C.free(unsafe.Pointer(id))

	res := Tags(id, 0)
	require.NotNil(t, res)
	require.Equal(t, C.arraylen(res, 3), C.int(3))

	// convert the C array to a Go Array so we can index it
	indexTag := (*[1<<29 - 1]*C.char)(unsafe.Pointer(res))[:4:4] // hardcoded expected length
	assert.Equal(t, C.GoString(indexTag[0]), "tag1")
	assert.Equal(t, C.GoString(indexTag[1]), "tag2")
	assert.Equal(t, C.GoString(indexTag[2]), "tag3")
	assert.Equal(t, unsafe.Pointer(indexTag[3]), unsafe.Pointer(nil))
}

func testTagsNull(t *testing.T) {
	sender := mocksender.NewMockSender(checkid.ID("testID"))
	logReceiver := optional.NewNoneOption[integrations.Component]()
	tagger := taggerimpl.SetupFakeTagger(t)
	tagger.SetTags(types.NewEntityID(types.ContainerID, "test"), "foo", nil, nil, nil, nil)
	release := scopeInitCheckContext(sender.GetSenderManager(), logReceiver, tagger)
	defer release()

	id := C.CString("container_id://test")
	defer C.free(unsafe.Pointer(id))

	res := Tags(id, 0)
	require.Nil(t, res)
}

func testTagsEmpty(t *testing.T) {
	sender := mocksender.NewMockSender(checkid.ID("testID"))
	logReceiver := optional.NewNoneOption[integrations.Component]()
	tagger := taggerimpl.SetupFakeTagger(t)
	tagger.SetTags(types.NewEntityID(types.ContainerID, "test"), "foo", []string{}, nil, nil, nil)
	release := scopeInitCheckContext(sender.GetSenderManager(), logReceiver, tagger)
	defer release()

	id := C.CString("container_id://test")
	defer C.free(unsafe.Pointer(id))

	res := Tags(id, 0)
	require.Nil(t, res)
}
