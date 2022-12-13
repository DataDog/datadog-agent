//go:build cgo && linux
// +build cgo,linux

package xccdf

/*
#cgo LDFLAGS: /opt/datadog-agent/embedded/lib/libopenscap.so
#cgo CFLAGS: -I/opt/datadog-agent/embedded/include/openscap
#include <stdio.h>
#include <stdlib.h>
#include <sys/stat.h>
#include <errno.h>
#include <xccdf_session.h>
*/
import "C"
import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (s *xccdfSession) EvaluateRule(rule string) ([]resources.ResolvedInstance, error) {
	if rule != "" {
		ruleCString := C.CString(rule)
		defer C.free(unsafe.Pointer(ruleCString))
		C.xccdf_session_set_rule(s.session, ruleCString)
	}

	/* Perform evaluation */
	if C.xccdf_session_evaluate(s.session) != 0 {
		return nil, fmt.Errorf("failed to evaluate session")
	}

	resIt := C.xccdf_session_get_rule_results(s.session)

	var instances []resources.ResolvedInstance
	for C.xccdf_rule_result_iterator_has_more(resIt) {
		res := C.xccdf_rule_result_iterator_next(resIt)
		ruleResult := C.xccdf_rule_result_get_result(res)
		ruleRef := C.xccdf_rule_result_get_idref(res)
		if ruleResult == 2 { // XCCDF_RESULT_FAIL
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
					"name":   C.GoString(ruleRef),
					"result": "failing",
				}), C.GoString(ruleRef), ""))
		} else if ruleResult == 4 || // XCCDF_RESULT_UNKNOWN
			ruleResult == 3 { // XCCDF_RESULT_ERROR
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
					"name":   C.GoString(ruleRef),
					"result": "error",
				}), C.GoString(ruleRef), ""))
		} else if ruleResult == 5 || // XCCDF_RESULT_NOT_APPLICABLE
			ruleResult == 6 || // XCCDF_RESULT_NOT_CHECKED
			ruleResult == 7 { // XCCDF_RESULT_NOT_SELECTED
		} else if ruleResult == 1 { // XCCDF_RESULT_PASS
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
					"name":   C.GoString(ruleRef),
					"result": "passed",
				}), C.GoString(ruleRef), ""))
		}
	}

	C.xccdf_rule_result_iterator_free(resIt)

	if C.xccdf_session_contains_fail_result(s.session) {
		log.Debugf("OVAL evaluation of rule %s returned errors", rule)
	} else {
		log.Debugf("Successfully evaluated OVAL rule %s", rule)
	}

	return instances, nil
}

func newXCCDFSession(xccdf, cpe string) (*xccdfSession, error) {
	xccdfCString := C.CString(xccdf)
	defer C.free(unsafe.Pointer(xccdfCString))

	session := C.xccdf_session_new(xccdfCString)
	if session == nil {
		return nil, fmt.Errorf("failed to create xccdf session for %s", xccdf)
	}

	log.Debugf("Created XCCDF session for %s", xccdf)

	cpeCString := C.CString(cpe)
	defer C.free(unsafe.Pointer(cpeCString))
	C.xccdf_session_set_user_cpe(session, cpeCString)

	productCpeCString := C.CString("cpe:/a:open-scap:oscap")
	defer C.free(unsafe.Pointer(productCpeCString))

	C.xccdf_session_set_product_cpe(session, productCpeCString)

	if errorCode := C.xccdf_session_load(session); errorCode != 0 {
		C.xccdf_session_free(session)
		return nil, errors.New("failed to load session")
	}

	return &xccdfSession{
		session: session,
	}, nil
}

func evalXCCDFRule(xccdf, cpe, profile, rule string) ([]resources.ResolvedInstance, error) {
	xccdfSession, err := newXCCDFSession(xccdf, cpe)
	if err != nil {
		return nil, err
	}
	defer xccdfSession.Close()

	if err := xccdfSession.SetProfile(profile); err != nil {
		return nil, err
	}

	return xccdfSession.EvaluateRule(rule)
}

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	configDir := e.ConfigDir()
	instances, err := evalXCCDFRule(filepath.Join(configDir, res.Xccdf.Name), filepath.Join(configDir, res.Xccdf.Cpe), res.Xccdf.Profile, res.Xccdf.Rule)
	if err != nil {
		return nil, err
	}

	return resources.NewResolvedInstances(instances), nil
}
