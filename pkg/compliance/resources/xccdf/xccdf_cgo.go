// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build libopenscap && cgo && linux
// +build libopenscap,cgo,linux

package xccdf

/*
#cgo LDFLAGS: /opt/datadog-agent/embedded/lib/libopenscap.so
#cgo CFLAGS: -I/opt/datadog-agent/embedded/include/openscap
#include <stdio.h>
#include <stdlib.h>
#include <sys/stat.h>
#include <errno.h>
#include <xccdf_session.h>
#include <oscap_error.h>
*/
import "C"
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	XCCDF_RESULT_PASS           = 1
	XCCDF_RESULT_FAIL           = 2
	XCCDF_RESULT_ERROR          = 3
	XCCDF_RESULT_UNKNOWN        = 4
	XCCDF_RESULT_NOT_APPLICABLE = 5
	XCCDF_RESULT_NOT_CHECKED    = 6
	XCCDF_RESULT_NOT_SELECTED   = 7
)

type xccdfSession struct {
	session *C.struct_xccdf_session
}

func (s *xccdfSession) Close() {
	C.xccdf_session_free(s.session)
}

func (s *xccdfSession) SetProfile(profile string) error {
	profileCString := C.CString(profile)
	defer C.free(unsafe.Pointer(profileCString))

	if !C.xccdf_session_set_profile_id(s.session, profileCString) {
		suffixMatchResult := C.xccdf_session_set_profile_id_by_suffix(s.session, profileCString)
		switch suffixMatchResult {
		case 1: // OSCAP_PROFILE_NO_MATCH
			return fmt.Errorf("missing profile %s", profile)
		case 2: // OSCAP_PROFILE_MULTIPLE_MATCHES
			return fmt.Errorf("%s matches multiple profiles", profile)
		default:
		}
	}

	return nil
}

func (s *xccdfSession) EvaluateRule(e env.Env, rule string) ([]resources.ResolvedInstance, error) {
	if config.IsContainerized() {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/host"
		}

		os.Setenv("OSCAP_PROBE_ROOT", hostRoot)
		defer os.Unsetenv("OSCAP_PROBE_ROOT")
	}

	if rule != "" {
		ruleCString := C.CString(rule)
		defer C.free(unsafe.Pointer(ruleCString))
		C.xccdf_session_set_rule(s.session, ruleCString)
	}

	/* Perform evaluation */
	if C.xccdf_session_evaluate(s.session) != 0 {
		return nil, getFullError("failed to evaluate session")
	}

	resIt := C.xccdf_session_get_rule_results(s.session)

	var instances []resources.ResolvedInstance
	for C.xccdf_rule_result_iterator_has_more(resIt) {
		res := C.xccdf_rule_result_iterator_next(resIt)
		ruleResult := C.xccdf_rule_result_get_result(res)
		ruleRef := C.xccdf_rule_result_get_idref(res)
		result := ""
		switch ruleResult {
		case XCCDF_RESULT_PASS:
			result = "passed"
		case XCCDF_RESULT_FAIL:
			result = "failing"
		case XCCDF_RESULT_ERROR, XCCDF_RESULT_UNKNOWN:
			result = "error"
		case XCCDF_RESULT_NOT_APPLICABLE, XCCDF_RESULT_NOT_CHECKED, XCCDF_RESULT_NOT_SELECTED:
		}
		if result != "" {
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
					"name":   e.Hostname(),
					"result": result,
				}), C.GoString(ruleRef), ""))
		}
	}

	C.xccdf_rule_result_iterator_free(resIt)

	if C.xccdf_session_contains_fail_result(s.session) {
		log.Debugf("OVAL evaluation of rule %s returned failures or errors", rule)
	} else {
		log.Debugf("Successfully evaluated OVAL rule %s", rule)
	}

	return instances, nil
}

func newXCCDFSession(xccdf string) (*xccdfSession, error) {
	xccdfCString := C.CString(xccdf)
	defer C.free(unsafe.Pointer(xccdfCString))

	session := C.xccdf_session_new(xccdfCString)
	if session == nil {
		return nil, getFullError(fmt.Sprintf("failed to load session for %s", xccdf))
	}

	log.Debugf("Created XCCDF session for %s", xccdf)
	productCpeCString := C.CString("cpe:/a:open-scap:oscap")
	defer C.free(unsafe.Pointer(productCpeCString))

	C.xccdf_session_set_product_cpe(session, productCpeCString)

	if errorCode := C.xccdf_session_load(session); errorCode != 0 {
		C.xccdf_session_free(session)
		return nil, getFullError("failed to load session")
	}

	return &xccdfSession{
		session: session,
	}, nil
}

func getFullError(errMsg string) error {
	return fmt.Errorf("%s: %s", errMsg, C.GoString(C.oscap_err_get_full_error()))
}

var mu sync.Mutex

func evalXCCDFRules(e env.Env, xccdf, profile string, rules []string) ([]resources.ResolvedInstance, error) {
	runtime.LockOSThread()

	mu.Lock()
	defer mu.Unlock()

	xccdfSession, err := newXCCDFSession(xccdf)
	if err != nil {
		return nil, err
	}
	defer xccdfSession.Close()

	if err := xccdfSession.SetProfile(profile); err != nil {
		return nil, err
	}

	return xccdfSession.EvaluateRule(e, rule)
}

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	configDir := e.ConfigDir()
	instances, err := evalXCCDFRule(e, filepath.Join(configDir, res.Xccdf.Name), res.Xccdf.Profile, res.Xccdf.Rule)
	if err != nil {
		return nil, err
	}

	return resources.NewResolvedInstances(instances), nil
}
