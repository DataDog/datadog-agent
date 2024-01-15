// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy

package sbom

import (
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func stringPtr(in string) *string {
	if in == "" {
		return nil
	}

	return &in
}

func strSliceDeref(in *[]string) []string {
	if in == nil {
		return nil
	}

	return *in
}

type inArrayElement interface {
	cyclonedx.Advisory |
		cyclonedx.AffectedVersions |
		cyclonedx.Affects |
		cyclonedx.Commit |
		cyclonedx.Component |
		cyclonedx.Composition |
		cyclonedx.Copyright |
		cyclonedx.DataClassification |
		cyclonedx.Dependency |
		cyclonedx.ExternalReference |
		cyclonedx.Hash |
		cyclonedx.Issue |
		cyclonedx.LicenseChoice |
		cyclonedx.Note |
		cyclonedx.OrganizationalContact |
		cyclonedx.OrganizationalEntity |
		cyclonedx.Patch |
		cyclonedx.Property |
		cyclonedx.Service |
		cyclonedx.Tool |
		cyclonedx.Vulnerability |
		cyclonedx.VulnerabilityRating |
		cyclonedx.VulnerabilityReference |
		string
}

type outArrayElement interface {
	cyclonedx_v1_4.Advisory |
		cyclonedx_v1_4.Commit |
		cyclonedx_v1_4.Component |
		cyclonedx_v1_4.Composition |
		cyclonedx_v1_4.DataClassification |
		cyclonedx_v1_4.Dependency |
		cyclonedx_v1_4.EvidenceCopyright |
		cyclonedx_v1_4.ExternalReference |
		cyclonedx_v1_4.Hash |
		cyclonedx_v1_4.Issue |
		cyclonedx_v1_4.LicenseChoice |
		cyclonedx_v1_4.Note |
		cyclonedx_v1_4.OrganizationalContact |
		cyclonedx_v1_4.OrganizationalEntity |
		cyclonedx_v1_4.Patch |
		cyclonedx_v1_4.Property |
		cyclonedx_v1_4.Service |
		cyclonedx_v1_4.Tool |
		cyclonedx_v1_4.Vulnerability |
		cyclonedx_v1_4.VulnerabilityAffectedVersions |
		cyclonedx_v1_4.VulnerabilityAffects |
		cyclonedx_v1_4.VulnerabilityRating |
		cyclonedx_v1_4.VulnerabilityReference
}

func convertArray[In inArrayElement, Out outArrayElement](in *[]In, convert func(*In) *Out) (out []*Out) {
	if in == nil {
		return nil
	}

	out = make([]*Out, 0, len(*in))
	for _, e := range *in {
		out = append(out, convert(&e))
	}
	return out
}

func convertAdvisory(in *cyclonedx.Advisory) *cyclonedx_v1_4.Advisory {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Advisory{
		Title: stringPtr(in.Title),
		Url:   in.URL,
	}
}

func convertAttachedText(in *cyclonedx.AttachedText) *cyclonedx_v1_4.AttachedText {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.AttachedText{
		ContentType: stringPtr(in.ContentType),
		Encoding:    stringPtr(in.Encoding),
		Value:       in.Content,
	}
}

func convertBOM(in *cyclonedx.BOM) *cyclonedx_v1_4.Bom {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Bom{
		SpecVersion:        in.SpecVersion.String(),
		Version:            pointer.Ptr(int32(in.Version)),
		SerialNumber:       stringPtr(in.SerialNumber),
		Metadata:           convertMetadata(in.Metadata),
		Components:         convertArray(in.Components, convertComponent),
		Services:           convertArray(in.Services, convertService),
		ExternalReferences: convertArray(in.ExternalReferences, convertExternalReference),
		Dependencies:       convertArray(in.Dependencies, convertDependency),
		Compositions:       convertArray(in.Compositions, convertComposition),
		Vulnerabilities:    convertArray(in.Vulnerabilities, convertVulnerability),
	}
}

func convertCommit(in *cyclonedx.Commit) *cyclonedx_v1_4.Commit {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Commit{
		Uid:       stringPtr(in.UID),
		Url:       stringPtr(in.URL),
		Author:    convertIdentifiableAction(in.Author),
		Committer: convertIdentifiableAction(in.Committer),
		Message:   stringPtr(in.Message),
	}
}

func convertComponent(in *cyclonedx.Component) *cyclonedx_v1_4.Component {
	if in == nil {
		return nil
	}

	var evidence []*cyclonedx_v1_4.Evidence
	if in.Evidence != nil {
		evidence = []*cyclonedx_v1_4.Evidence{convertEvidence(in.Evidence)}
	}

	return &cyclonedx_v1_4.Component{
		Type:               convertComponentType(in.Type),
		MimeType:           stringPtr(in.MIMEType),
		BomRef:             stringPtr(in.BOMRef),
		Supplier:           convertOrganizationalEntity(in.Supplier),
		Author:             stringPtr(in.Author),
		Publisher:          stringPtr(in.Publisher),
		Group:              stringPtr(in.Group),
		Name:               in.Name,
		Version:            in.Version,
		Description:        stringPtr(in.Description),
		Scope:              convertScope(in.Scope),
		Hashes:             convertArray(in.Hashes, convertHash),
		Licenses:           convertArray(castLicenses(in.Licenses), convertLicenseChoice),
		Copyright:          stringPtr(in.Copyright),
		Cpe:                stringPtr(in.CPE),
		Purl:               stringPtr(in.PackageURL),
		Swid:               convertSwid(in.SWID),
		Modified:           in.Modified,
		Pedigree:           convertPedigree(in.Pedigree),
		ExternalReferences: convertArray(in.ExternalReferences, convertExternalReference),
		Components:         convertArray(in.Components, convertComponent),
		Properties:         convertArray(in.Properties, convertProperty),
		Evidence:           evidence,
		ReleaseNotes:       convertReleaseNotes(in.ReleaseNotes),
	}
}

func convertComponentType(in cyclonedx.ComponentType) cyclonedx_v1_4.Classification {
	switch in {
	case cyclonedx.ComponentTypeApplication:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_APPLICATION
	case cyclonedx.ComponentTypeContainer:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_CONTAINER
	case cyclonedx.ComponentTypeDevice:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_DEVICE
	case cyclonedx.ComponentTypeFile:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_FILE
	case cyclonedx.ComponentTypeFirmware:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_FIRMWARE
	case cyclonedx.ComponentTypeFramework:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_FRAMEWORK
	case cyclonedx.ComponentTypeLibrary:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_LIBRARY
	case cyclonedx.ComponentTypeOS:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_OPERATING_SYSTEM
	default:
		return cyclonedx_v1_4.Classification_CLASSIFICATION_NULL
	}
}

func convertComposition(in *cyclonedx.Composition) (out *cyclonedx_v1_4.Composition) {
	if in == nil {
		return nil
	}

	out = &cyclonedx_v1_4.Composition{
		Aggregate: convertCompositionAggregate(in.Aggregate),
	}

	if in.Assemblies != nil {
		out.Assemblies = *(*[]string)(unsafe.Pointer(in.Assemblies))
	}

	if in.Dependencies != nil {
		out.Dependencies = *(*[]string)(unsafe.Pointer(in.Dependencies))
	}

	return out
}

func convertCompositionAggregate(in cyclonedx.CompositionAggregate) cyclonedx_v1_4.Aggregate {
	switch in {
	case cyclonedx.CompositionAggregateComplete:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_COMPLETE
	case cyclonedx.CompositionAggregateIncomplete:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_INCOMPLETE
	case cyclonedx.CompositionAggregateIncompleteFirstPartyOnly:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_INCOMPLETE_FIRST_PARTY_ONLY
	case cyclonedx.CompositionAggregateIncompleteThirdPartyOnly:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_INCOMPLETE_THIRD_PARTY_ONLY
	case cyclonedx.CompositionAggregateUnknown:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_UNKNOWN
	case cyclonedx.CompositionAggregateNotSpecified:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_NOT_SPECIFIED
	default:
		return cyclonedx_v1_4.Aggregate_AGGREGATE_NOT_SPECIFIED
	}
}

func convertCopyright(in *cyclonedx.Copyright) *cyclonedx_v1_4.EvidenceCopyright {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.EvidenceCopyright{
		Text: in.Text,
	}
}

func convertDataClassification(in *cyclonedx.DataClassification) *cyclonedx_v1_4.DataClassification {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.DataClassification{
		Flow:  convertDataFlow(in.Flow),
		Value: in.Classification,
	}
}

func convertDataFlow(in cyclonedx.DataFlow) cyclonedx_v1_4.DataFlow {
	switch in {
	case cyclonedx.DataFlowBidirectional:
		return cyclonedx_v1_4.DataFlow_DATA_FLOW_BI_DIRECTIONAL
	case cyclonedx.DataFlowInbound:
		return cyclonedx_v1_4.DataFlow_DATA_FLOW_INBOUND
	case cyclonedx.DataFlowOutbound:
		return cyclonedx_v1_4.DataFlow_DATA_FLOW_OUTBOUND
	case cyclonedx.DataFlowUnknown:
		return cyclonedx_v1_4.DataFlow_DATA_FLOW_UNKNOWN
	default:
		return cyclonedx_v1_4.DataFlow_DATA_FLOW_NULL
	}
}

func convertDependency(in *cyclonedx.Dependency) *cyclonedx_v1_4.Dependency {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Dependency{
		Ref:          in.Ref,
		Dependencies: convertArray(in.Dependencies, convertDependencyString),
	}
}

func convertDependencyString(in *string) *cyclonedx_v1_4.Dependency {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Dependency{
		Ref: *in,
	}
}

func convertDiff(in *cyclonedx.Diff) *cyclonedx_v1_4.Diff {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Diff{
		Text: convertAttachedText(in.Text),
		Url:  stringPtr(in.URL),
	}
}

func convertEvidence(in *cyclonedx.Evidence) *cyclonedx_v1_4.Evidence {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Evidence{
		Licenses:  convertArray(castLicenses(in.Licenses), convertLicenseChoice),
		Copyright: convertArray(in.Copyright, convertCopyright),
	}
}

func convertExternalReference(in *cyclonedx.ExternalReference) *cyclonedx_v1_4.ExternalReference {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.ExternalReference{
		Type:    convertExternalReferenceType(in.Type),
		Url:     in.URL,
		Comment: stringPtr(in.Comment),
		Hashes:  convertArray(in.Hashes, convertHash),
	}
}

func convertExternalReferenceType(in cyclonedx.ExternalReferenceType) cyclonedx_v1_4.ExternalReferenceType {
	switch in {
	case cyclonedx.ERTypeAdvisories:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_ADVISORIES
	case cyclonedx.ERTypeBOM:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_BOM
	case cyclonedx.ERTypeBuildMeta:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_BUILD_META
	case cyclonedx.ERTypeBuildSystem:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_BUILD_SYSTEM
	case cyclonedx.ERTypeChat:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_CHAT
	case cyclonedx.ERTypeDistribution:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_DISTRIBUTION
	case cyclonedx.ERTypeDocumentation:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_DOCUMENTATION
	case cyclonedx.ERTypeLicense:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_LICENSE
	case cyclonedx.ERTypeMailingList:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_MAILING_LIST
	case cyclonedx.ERTypeOther:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_OTHER
	case cyclonedx.ERTypeIssueTracker:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_ISSUE_TRACKER
	case cyclonedx.ERTypeReleaseNotes:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_OTHER // ??
	case cyclonedx.ERTypeSocial:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_SOCIAL
	case cyclonedx.ERTypeSupport:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_SUPPORT
	case cyclonedx.ERTypeVCS:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_VCS
	case cyclonedx.ERTypeWebsite:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_WEBSITE
	default:
		return cyclonedx_v1_4.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_OTHER
	}
}

func convertHash(in *cyclonedx.Hash) *cyclonedx_v1_4.Hash {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Hash{
		Alg:   convertHashAlgo(in.Algorithm),
		Value: in.Value,
	}
}

func convertHashAlgo(in cyclonedx.HashAlgorithm) cyclonedx_v1_4.HashAlg {
	switch in {
	case cyclonedx.HashAlgoMD5:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_MD_5
	case cyclonedx.HashAlgoSHA1:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_SHA_1
	case cyclonedx.HashAlgoSHA256:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_SHA_256
	case cyclonedx.HashAlgoSHA384:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_SHA_384
	case cyclonedx.HashAlgoSHA512:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_SHA_512
	case cyclonedx.HashAlgoSHA3_256:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_SHA_3_256
	case cyclonedx.HashAlgoSHA3_512:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_SHA_3_512
	case cyclonedx.HashAlgoBlake2b_256:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_BLAKE_2_B_256
	case cyclonedx.HashAlgoBlake2b_384:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_BLAKE_2_B_384
	case cyclonedx.HashAlgoBlake2b_512:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_BLAKE_2_B_512
	case cyclonedx.HashAlgoBlake3:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_BLAKE_3
	default:
		return cyclonedx_v1_4.HashAlg_HASH_ALG_NULL
	}
}

func convertIdentifiableAction(in *cyclonedx.IdentifiableAction) *cyclonedx_v1_4.IdentifiableAction {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.IdentifiableAction{
		Timestamp: convertTimestamp(in.Timestamp),
		Name:      stringPtr(in.Name),
		Email:     stringPtr(in.Email),
	}
}

func convertImpactAnalysisJustification(in cyclonedx.ImpactAnalysisJustification) *cyclonedx_v1_4.ImpactAnalysisJustification {
	if in == "" {
		return nil
	}

	switch in {
	case cyclonedx.IAJCodeNotPresent:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_CODE_NOT_PRESENT)
	case cyclonedx.IAJCodeNotReachable:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_CODE_NOT_REACHABLE)
	case cyclonedx.IAJRequiresConfiguration:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_REQUIRES_CONFIGURATION)
	case cyclonedx.IAJRequiresDependency:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_REQUIRES_DEPENDENCY)
	case cyclonedx.IAJRequiresEnvironment:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_REQUIRES_ENVIRONMENT)
	case cyclonedx.IAJProtectedByCompiler:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_PROTECTED_BY_COMPILER)
	case cyclonedx.IAJProtectedAtRuntime:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_PROTECTED_AT_RUNTIME)
	case cyclonedx.IAJProtectedAtPerimeter:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_PROTECTED_AT_PERIMETER)
	case cyclonedx.IAJProtectedByMitigatingControl:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_PROTECTED_BY_MITIGATING_CONTROL)
	default:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisJustification_IMPACT_ANALYSIS_JUSTIFICATION_NULL)
	}
}

func convertImpactAnalysisResponse(in cyclonedx.ImpactAnalysisResponse) cyclonedx_v1_4.VulnerabilityResponse {
	switch in {
	case cyclonedx.IARCanNotFix:
		return cyclonedx_v1_4.VulnerabilityResponse_VULNERABILITY_RESPONSE_CAN_NOT_FIX
	case cyclonedx.IARWillNotFix:
		return cyclonedx_v1_4.VulnerabilityResponse_VULNERABILITY_RESPONSE_WILL_NOT_FIX
	case cyclonedx.IARUpdate:
		return cyclonedx_v1_4.VulnerabilityResponse_VULNERABILITY_RESPONSE_UPDATE
	case cyclonedx.IARRollback:
		return cyclonedx_v1_4.VulnerabilityResponse_VULNERABILITY_RESPONSE_ROLLBACK
	case cyclonedx.IARWorkaroundAvailable:
		return cyclonedx_v1_4.VulnerabilityResponse_VULNERABILITY_RESPONSE_WORKAROUND_AVAILABLE
	default:
		return cyclonedx_v1_4.VulnerabilityResponse_VULNERABILITY_RESPONSE_NULL
	}
}

func convertImpactAnalysisState(in cyclonedx.ImpactAnalysisState) *cyclonedx_v1_4.ImpactAnalysisState {
	if in == "" {
		return nil
	}

	switch in {
	case cyclonedx.IASResolved:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_RESOLVED)
	case cyclonedx.IASResolvedWithPedigree:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_RESOLVED_WITH_PEDIGREE)
	case cyclonedx.IASExploitable:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_EXPLOITABLE)
	case cyclonedx.IASInTriage:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_IN_TRIAGE)
	case cyclonedx.IASFalsePositive:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_FALSE_POSITIVE)
	case cyclonedx.IASNotAffected:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_NOT_AFFECTED)
	default:
		return pointer.Ptr(cyclonedx_v1_4.ImpactAnalysisState_IMPACT_ANALYSIS_STATE_NULL)
	}
}

func convertIssue(in *cyclonedx.Issue) *cyclonedx_v1_4.Issue {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Issue{
		Type:        convertIssueType(in.Type),
		Id:          stringPtr(in.ID),
		Name:        stringPtr(in.Name),
		Description: stringPtr(in.Description),
		Source:      convertSource(in.Source),
		References:  strSliceDeref(in.References),
	}
}

func convertIssueType(in cyclonedx.IssueType) cyclonedx_v1_4.IssueClassification {
	switch in {
	case cyclonedx.IssueTypeDefect:
		return cyclonedx_v1_4.IssueClassification_ISSUE_CLASSIFICATION_DEFECT
	case cyclonedx.IssueTypeEnhancement:
		return cyclonedx_v1_4.IssueClassification_ISSUE_CLASSIFICATION_ENHANCEMENT
	case cyclonedx.IssueTypeSecurity:
		return cyclonedx_v1_4.IssueClassification_ISSUE_CLASSIFICATION_SECURITY
	default:
		return cyclonedx_v1_4.IssueClassification_ISSUE_CLASSIFICATION_NULL
	}
}

func castLicenses(in *cyclonedx.Licenses) *[]cyclonedx.LicenseChoice {
	if in == nil {
		return nil
	}

	var l []cyclonedx.LicenseChoice = *in
	return &l
}

func convertLicense(in *cyclonedx.License) (out *cyclonedx_v1_4.License) {
	if in == nil {
		return nil
	}

	out = &cyclonedx_v1_4.License{
		Text: convertAttachedText(in.Text),
		Url:  stringPtr(in.URL),
	}

	if in.ID != "" {
		out.License = &cyclonedx_v1_4.License_Id{
			Id: in.ID,
		}
	}

	if in.Name != "" {
		out.License = &cyclonedx_v1_4.License_Name{
			Name: in.Name,
		}
	}

	return out
}

func convertLicenseChoice(in *cyclonedx.LicenseChoice) *cyclonedx_v1_4.LicenseChoice {
	if in == nil {
		return nil
	}

	if in.License != nil {
		return &cyclonedx_v1_4.LicenseChoice{
			Choice: &cyclonedx_v1_4.LicenseChoice_License{
				License: convertLicense(in.License),
			},
		}
	}

	if in.Expression != "" {
		return &cyclonedx_v1_4.LicenseChoice{
			Choice: &cyclonedx_v1_4.LicenseChoice_Expression{
				Expression: in.Expression,
			},
		}
	}

	return nil
}

func convertMetadata(in *cyclonedx.Metadata) *cyclonedx_v1_4.Metadata {
	if in == nil {
		return nil
	}

	var licenses *cyclonedx_v1_4.LicenseChoice
	if in.Licenses != nil && len(*in.Licenses) > 0 {
		licenses = convertLicenseChoice(&(*in.Licenses)[0])
	}

	return &cyclonedx_v1_4.Metadata{
		Timestamp:   convertTimestamp(in.Timestamp),
		Tools:       convertArray(in.Tools, convertTool),
		Authors:     convertArray(in.Authors, convertOrganizationalContact),
		Component:   convertComponent(in.Component),
		Manufacture: convertOrganizationalEntity(in.Manufacture),
		Supplier:    convertOrganizationalEntity(in.Supplier),
		Licenses:    licenses,
		Properties:  convertArray(in.Properties, convertProperty),
	}
}

func convertNote(in *cyclonedx.Note) *cyclonedx_v1_4.Note {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Note{
		Locale: stringPtr(in.Locale),
		Text:   convertAttachedText(&in.Text),
	}
}

func convertOrganizationalContact(in *cyclonedx.OrganizationalContact) *cyclonedx_v1_4.OrganizationalContact {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.OrganizationalContact{
		Name:  stringPtr(in.Name),
		Email: stringPtr(in.Email),
		Phone: stringPtr(in.Phone),
	}
}

func convertOrganizationalEntity(in *cyclonedx.OrganizationalEntity) *cyclonedx_v1_4.OrganizationalEntity {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.OrganizationalEntity{
		Name:    stringPtr(in.Name),
		Url:     strSliceDeref(in.URL),
		Contact: convertArray(in.Contact, convertOrganizationalContact),
	}
}

func convertPatch(in *cyclonedx.Patch) *cyclonedx_v1_4.Patch {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Patch{
		Type:     convertPatchType(in.Type),
		Diff:     convertDiff(in.Diff),
		Resolves: convertArray(in.Resolves, convertIssue),
	}
}

func convertPatchType(in cyclonedx.PatchType) cyclonedx_v1_4.PatchClassification {
	switch in {
	case cyclonedx.PatchTypeBackport:
		return cyclonedx_v1_4.PatchClassification_PATCH_CLASSIFICATION_BACKPORT
	case cyclonedx.PatchTypeCherryPick:
		return cyclonedx_v1_4.PatchClassification_PATCH_CLASSIFICATION_CHERRY_PICK
	case cyclonedx.PatchTypeMonkey:
		return cyclonedx_v1_4.PatchClassification_PATCH_CLASSIFICATION_MONKEY
	case cyclonedx.PatchTypeUnofficial:
		return cyclonedx_v1_4.PatchClassification_PATCH_CLASSIFICATION_UNOFFICIAL
	default:
		return cyclonedx_v1_4.PatchClassification_PATCH_CLASSIFICATION_NULL
	}
}

func convertPedigree(in *cyclonedx.Pedigree) *cyclonedx_v1_4.Pedigree {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Pedigree{
		Ancestors:   convertArray(in.Ancestors, convertComponent),
		Descendants: convertArray(in.Descendants, convertComponent),
		Variants:    convertArray(in.Variants, convertComponent),
		Commits:     convertArray(in.Commits, convertCommit),
		Patches:     convertArray(in.Patches, convertPatch),
		Notes:       stringPtr(in.Notes),
	}
}

func convertProperty(in *cyclonedx.Property) *cyclonedx_v1_4.Property {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Property{
		Name:  in.Name,
		Value: stringPtr(in.Value),
	}
}

func convertReleaseNotes(in *cyclonedx.ReleaseNotes) *cyclonedx_v1_4.ReleaseNotes {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.ReleaseNotes{
		Type:          in.Type,
		Title:         stringPtr(in.Title),
		FeaturedImage: stringPtr(in.FeaturedImage),
		SocialImage:   stringPtr(in.SocialImage),
		Description:   stringPtr(in.Description),
		Timestamp:     convertTimestamp(in.Timestamp),
		Aliases:       strSliceDeref(in.Aliases),
		Tags:          strSliceDeref(in.Tags),
		Resolves:      convertArray(in.Resolves, convertIssue),
		Notes:         convertArray(in.Notes, convertNote),
		Properties:    convertArray(in.Properties, convertProperty),
	}
}

func convertScope(in cyclonedx.Scope) *cyclonedx_v1_4.Scope {
	if in == "" {
		return nil
	}

	switch in {
	case cyclonedx.ScopeExcluded:
		return pointer.Ptr(cyclonedx_v1_4.Scope_SCOPE_EXCLUDED)
	case cyclonedx.ScopeOptional:
		return pointer.Ptr(cyclonedx_v1_4.Scope_SCOPE_OPTIONAL)
	case cyclonedx.ScopeRequired:
		return pointer.Ptr(cyclonedx_v1_4.Scope_SCOPE_REQUIRED)
	default:
		return pointer.Ptr(cyclonedx_v1_4.Scope_SCOPE_UNSPECIFIED)
	}
}

func convertScoringMethod(in cyclonedx.ScoringMethod) *cyclonedx_v1_4.ScoreMethod {
	if in == "" {
		return nil
	}

	switch in {
	case cyclonedx.ScoringMethodOther:
		return pointer.Ptr(cyclonedx_v1_4.ScoreMethod_SCORE_METHOD_OTHER)
	case cyclonedx.ScoringMethodCVSSv2:
		return pointer.Ptr(cyclonedx_v1_4.ScoreMethod_SCORE_METHOD_CVSSV2)
	case cyclonedx.ScoringMethodCVSSv3:
		return pointer.Ptr(cyclonedx_v1_4.ScoreMethod_SCORE_METHOD_CVSSV3)
	case cyclonedx.ScoringMethodCVSSv31:
		return pointer.Ptr(cyclonedx_v1_4.ScoreMethod_SCORE_METHOD_CVSSV31)
	case cyclonedx.ScoringMethodOWASP:
		return pointer.Ptr(cyclonedx_v1_4.ScoreMethod_SCORE_METHOD_OWASP)
	default:
		return pointer.Ptr(cyclonedx_v1_4.ScoreMethod_SCORE_METHOD_NULL)
	}
}

func convertService(in *cyclonedx.Service) *cyclonedx_v1_4.Service {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Service{
		BomRef:             stringPtr(in.BOMRef),
		Provider:           convertOrganizationalEntity(in.Provider),
		Group:              stringPtr(in.Group),
		Name:               in.Name,
		Version:            stringPtr(in.Version),
		Description:        stringPtr(in.Description),
		Endpoints:          strSliceDeref(in.Endpoints),
		Authenticated:      in.Authenticated,
		XTrustBoundary:     in.CrossesTrustBoundary,
		Data:               convertArray(in.Data, convertDataClassification),
		Licenses:           convertArray(castLicenses(in.Licenses), convertLicenseChoice),
		ExternalReferences: convertArray(in.ExternalReferences, convertExternalReference),
		Services:           convertArray(in.Services, convertService),
		Properties:         convertArray(in.Properties, convertProperty),
		ReleaseNotes:       convertReleaseNotes(in.ReleaseNotes),
	}
}

func convertSeverity(in cyclonedx.Severity) *cyclonedx_v1_4.Severity {
	if in == "" {
		return nil
	}

	switch in {
	case cyclonedx.SeverityUnknown:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_UNKNOWN)
	case cyclonedx.SeverityNone:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_NONE)
	case cyclonedx.SeverityInfo:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_INFO)
	case cyclonedx.SeverityLow:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_LOW)
	case cyclonedx.SeverityMedium:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_MEDIUM)
	case cyclonedx.SeverityHigh:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_HIGH)
	case cyclonedx.SeverityCritical:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_CRITICAL)
	default:
		return pointer.Ptr(cyclonedx_v1_4.Severity_SEVERITY_UNKNOWN)
	}
}

func convertSource(in *cyclonedx.Source) *cyclonedx_v1_4.Source {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Source{
		Name: stringPtr(in.Name),
		Url:  stringPtr(in.URL),
	}
}

func convertSwid(in *cyclonedx.SWID) *cyclonedx_v1_4.Swid {
	if in == nil {
		return nil
	}

	var tagVersion *int32
	if in.TagVersion != nil {
		tagVersion = pointer.Ptr(int32(*in.TagVersion))
	}

	return &cyclonedx_v1_4.Swid{
		TagId:      in.TagID,
		Name:       in.Name,
		Version:    stringPtr(in.Version),
		TagVersion: tagVersion,
		Patch:      in.Patch,
		Text:       convertAttachedText(in.Text),
		Url:        stringPtr(in.URL),
	}
}

func convertTimestamp(in string) *timestamppb.Timestamp {
	ts, err := time.Parse(time.RFC3339, in)
	if err != nil {
		return nil
	}

	return timestamppb.New(ts)
}

func convertDuration(in time.Duration) *durationpb.Duration {
	return durationpb.New(in)
}

func convertTool(in *cyclonedx.Tool) *cyclonedx_v1_4.Tool {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.Tool{
		Vendor:             stringPtr(in.Vendor),
		Name:               stringPtr(in.Name),
		Version:            stringPtr(in.Version),
		Hashes:             convertArray(in.Hashes, convertHash),
		ExternalReferences: convertArray(in.ExternalReferences, convertExternalReference),
	}
}

func convertVulnerability(in *cyclonedx.Vulnerability) *cyclonedx_v1_4.Vulnerability {
	if in == nil {
		return nil
	}

	var cwes []int32
	if in.CWEs != nil {
		cwes = make([]int32, len(*in.CWEs))
		for i := range *in.CWEs {
			cwes[i] = int32((*in.CWEs)[i])
		}
	}

	return &cyclonedx_v1_4.Vulnerability{
		BomRef:         stringPtr(in.BOMRef),
		Id:             stringPtr(in.ID),
		Source:         convertSource(in.Source),
		References:     convertArray(in.References, convertVulnerabilityReference),
		Ratings:        convertArray(in.Ratings, convertVulnerabilityRating),
		Cwes:           cwes,
		Description:    stringPtr(in.Description),
		Detail:         stringPtr(in.Detail),
		Recommendation: stringPtr(in.Recommendation),
		Advisories:     convertArray(in.Advisories, convertAdvisory),
		Created:        convertTimestamp(in.Created),
		Published:      convertTimestamp(in.Published),
		Updated:        convertTimestamp(in.Updated),
		Credits:        convertVulnerabilityCredits(in.Credits),
		Tools:          convertArray(in.Tools, convertTool),
		Analysis:       convertVulnerabilityAnalysis(in.Analysis),
		Affects:        convertArray(in.Affects, convertVulnerabilityAffects),
		Properties:     convertArray(in.Properties, convertProperty),
	}
}

func convertVulnerabilityAffectedStatus(in cyclonedx.VulnerabilityStatus) *cyclonedx_v1_4.VulnerabilityAffectedStatus {
	if in == "" {
		return nil
	}

	switch in {
	case cyclonedx.VulnerabilityStatusUnknown:
		return pointer.Ptr(cyclonedx_v1_4.VulnerabilityAffectedStatus_VULNERABILITY_AFFECTED_STATUS_UNKNOWN)
	case cyclonedx.VulnerabilityStatusAffected:
		return pointer.Ptr(cyclonedx_v1_4.VulnerabilityAffectedStatus_VULNERABILITY_AFFECTED_STATUS_AFFECTED)
	case cyclonedx.VulnerabilityStatusNotAffected:
		return pointer.Ptr(cyclonedx_v1_4.VulnerabilityAffectedStatus_VULNERABILITY_AFFECTED_STATUS_NOT_AFFECTED)
	default:
		return pointer.Ptr(cyclonedx_v1_4.VulnerabilityAffectedStatus_VULNERABILITY_AFFECTED_STATUS_UNKNOWN)
	}
}

func convertVulnerabilityAffectedVersions(in *cyclonedx.AffectedVersions) (out *cyclonedx_v1_4.VulnerabilityAffectedVersions) {
	if in == nil {
		return nil
	}

	out = &cyclonedx_v1_4.VulnerabilityAffectedVersions{
		Status: convertVulnerabilityAffectedStatus(in.Status),
	}

	if in.Version != "" {
		out.Choice = &cyclonedx_v1_4.VulnerabilityAffectedVersions_Version{
			Version: in.Version,
		}
	}

	if in.Range != "" {
		out.Choice = &cyclonedx_v1_4.VulnerabilityAffectedVersions_Range{
			Range: in.Range,
		}
	}

	return out
}

func convertVulnerabilityAffects(in *cyclonedx.Affects) *cyclonedx_v1_4.VulnerabilityAffects {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.VulnerabilityAffects{
		Ref:      in.Ref,
		Versions: convertArray(in.Range, convertVulnerabilityAffectedVersions),
	}
}

func convertVulnerabilityAnalysis(in *cyclonedx.VulnerabilityAnalysis) *cyclonedx_v1_4.VulnerabilityAnalysis {
	if in == nil {
		return nil
	}

	var response []cyclonedx_v1_4.VulnerabilityResponse
	if in.Response != nil {
		response = make([]cyclonedx_v1_4.VulnerabilityResponse, 0, len(*in.Response))
		for _, e := range *in.Response {
			response = append(response, convertImpactAnalysisResponse(e))
		}
	}

	return &cyclonedx_v1_4.VulnerabilityAnalysis{
		State:         convertImpactAnalysisState(in.State),
		Justification: convertImpactAnalysisJustification(in.Justification),
		Response:      response,
		Detail:        stringPtr(in.Detail),
	}
}

func convertVulnerabilityCredits(in *cyclonedx.Credits) *cyclonedx_v1_4.VulnerabilityCredits {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.VulnerabilityCredits{
		Organizations: convertArray(in.Organizations, convertOrganizationalEntity),
		Individuals:   convertArray(in.Individuals, convertOrganizationalContact),
	}
}

func convertVulnerabilityRating(in *cyclonedx.VulnerabilityRating) *cyclonedx_v1_4.VulnerabilityRating {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.VulnerabilityRating{
		Source:        convertSource(in.Source),
		Score:         in.Score,
		Severity:      convertSeverity(in.Severity),
		Method:        convertScoringMethod(in.Method),
		Vector:        stringPtr(in.Vector),
		Justification: stringPtr(in.Justification),
	}
}

func convertVulnerabilityReference(in *cyclonedx.VulnerabilityReference) *cyclonedx_v1_4.VulnerabilityReference {
	if in == nil {
		return nil
	}

	return &cyclonedx_v1_4.VulnerabilityReference{
		Id:     stringPtr(in.ID),
		Source: convertSource(in.Source),
	}
}
