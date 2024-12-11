// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package autoscaling

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	core "k8s.io/client-go/testing"
)

// CheckAction verifies that expected and actual actions are equal and both have
// same attached resources
func CheckAction(t *testing.T, expected, actual core.Action) {
	t.Helper()
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		diff := objectDiff(expObject, object)
		assert.Empty(t, diff, "## + is content from actual, ## - is content from expected")
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		diff := objectDiff(expObject, object)
		assert.Empty(t, diff, "## + is content from actual, ## - is content from expected")
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		diff := objectDiff(expPatch, patch)
		assert.Empty(t, diff, "## + is content from actual, ## - is content from expected")
	case core.DeleteActionImpl:
		e, _ := expected.(core.DeleteActionImpl)
		expDeletedItem := e.GetNamespace() + "/" + e.GetName()
		deletedItem := a.GetNamespace() + "/" + e.GetName()

		if deletedItem != expDeletedItem {
			t.Errorf("Action %s %s has wrong target, exp: %s, actual: %s", a.GetVerb(), a.GetResource().Resource, expDeletedItem, deletedItem)
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// FilterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func FilterInformerActions(actions []core.Action, resourceName string) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", resourceName) ||
				action.Matches("watch", resourceName)) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func objectDiff(expected, actual any) string {
	return cmp.Diff(expected, actual)
}
