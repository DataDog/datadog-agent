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
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var sessionCache = map[string]*xccdfSession{}

const (
	XCCDF_RESULT_PASS           = 1
	XCCDF_RESULT_FAIL           = 2
	XCCDF_RESULT_ERROR          = 3
	XCCDF_RESULT_UNKNOWN        = 4
	XCCDF_RESULT_NOT_APPLICABLE = 5
	XCCDF_RESULT_NOT_CHECKED    = 6
	XCCDF_RESULT_NOT_SELECTED   = 7
)

var oscapSingleton = &oscapWrapper{
	in:  make(chan oscapIn),
	out: make(chan oscapOut),
}

func init() {
	go oscapSingleton.mainLoop()
}

type oscapIn struct {
	hostname string
	xccdf    string
	profile  string
	rules    []string
}

type oscapOut struct {
	instances []resources.ResolvedInstance
	err       error
}

type oscapWrapper struct {
	in  chan oscapIn
	out chan oscapOut
}

func (w *oscapWrapper) mainLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for arg := range w.in {
		instances, err := w.do(arg.hostname, arg.xccdf, arg.profile, arg.rules)
		w.out <- oscapOut{instances, err}
	}
}

func (w *oscapWrapper) do(hostname, xccdf, profile string, rules []string) ([]resources.ResolvedInstance, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("no rule to evaluate")
	}

	var err error
	xccdfSession := sessionCache[xccdf]
	if xccdfSession == nil {
		if xccdfSession, err = newXCCDFSession(xccdf); err != nil {
			return nil, err
		}
		sessionCache[xccdf] = xccdfSession
	}

	if err := xccdfSession.SetProfile(profile); err != nil {
		return nil, err
	}

	return xccdfSession.EvaluateRules(hostname, rules)
}

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

func (s *xccdfSession) EvaluateRules(hostname string, rules []string) ([]resources.ResolvedInstance, error) {
	if config.IsContainerized() {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/host"
		}

		os.Setenv("OSCAP_PROBE_ROOT", hostRoot)
		defer os.Unsetenv("OSCAP_PROBE_ROOT")
	}

	for _, rule := range rules {
		ruleCString := C.CString(rule)
		defer C.free(unsafe.Pointer(ruleCString))
		log.Tracef("adding rule for xccdf eval %q", rule)
		C.xccdf_session_add_rule(s.session, ruleCString)
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
		ruleRef := C.GoString(C.xccdf_rule_result_get_idref(res))
		result := ""
		switch ruleResult {
		case XCCDF_RESULT_PASS:
			result = "passed"
		case XCCDF_RESULT_FAIL:
			result = "failing"
		case XCCDF_RESULT_ERROR, XCCDF_RESULT_UNKNOWN:
			result = "error"
		case XCCDF_RESULT_NOT_APPLICABLE:
		case XCCDF_RESULT_NOT_CHECKED, XCCDF_RESULT_NOT_SELECTED:
		}
		if result != "" {
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
					"name":   hostname,
					"result": result,
					"rule":   ruleRef,
				}), hostname, "host"))
		}
	}
	C.xccdf_rule_result_iterator_free(resIt)

	if C.xccdf_session_contains_fail_result(s.session) {
		log.Debugf("OVAL evaluation of rules %v returned failures or errors", rules)
	} else {
		log.Debugf("Successfully evaluated OVAL rules %v", rules)
	}

	C.xccdf_session_result_free(s.session)

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

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	configDir := e.ConfigDir()

	var rules []string
	if res.Xccdf.Rule != "" {
		rules = []string{res.Xccdf.Rule}
	} else {
		rules = res.Xccdf.Rules
	}

	oscapSingleton.in <- oscapIn{
		hostname: e.Hostname(),
		xccdf:    filepath.Join(configDir, res.Xccdf.Name),
		profile:  res.Xccdf.Profile,
		rules:    rules,
	}
	result := <-oscapSingleton.out

	if result.err != nil {
		return nil, result.err
	}

	return resources.NewResolvedInstances(result.instances), nil
}
