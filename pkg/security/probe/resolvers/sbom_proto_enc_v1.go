// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package resolvers

import (
	"fmt"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	trivyMarshaler "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	types "github.com/aquasecurity/trivy/pkg/types"
	"github.com/golang/protobuf/ptypes/timestamp"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	ddCycloneDXProto "github.com/DataDog/datadog-agent/pkg/security/api/cyclonedx"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// ToSBOMMessage returns an *api.SBOMMessage instance from an SBOM instance
func (s *SBOM) ToSBOMMessage() (*api.SBOMMessage, error) {
	cycloneDXBOM, err := reportToDDCycloneDXProto(s.report)
	if err != nil {
		return nil, err
	}

	msg := &api.SBOMMessage{
		Host:        s.Host,
		Service:     s.Service,
		Source:      s.Source,
		Tags:        make([]string, len(s.Tags)),
		BOM:         cycloneDXBOM,
		ContainerID: s.ContainerID,
	}
	copy(msg.Tags, s.Tags)
	return msg, nil
}

func reportToDDCycloneDXProto(report *types.Report) (*ddCycloneDXProto.Bom, error) {
	marshaler := trivyMarshaler.NewMarshaler("")
	cycloneDXBom, err := marshaler.Marshal(*report)
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal report: %w", err)
	}
	return cycloneDXBomToProto(cycloneDXBom), nil
}

func cycloneDXBomToProto(bom *cyclonedx.BOM) *ddCycloneDXProto.Bom {
	if bom == nil {
		return nil
	}

	return &ddCycloneDXProto.Bom{
		SpecVersion:        bom.SpecVersion,
		Version:            int32(bom.Version),
		SerialNumber:       bom.SerialNumber,
		Metadata:           cycloneDXMetadataToProto(bom.Metadata),
		Components:         cycloneDXComponentsToProto(bom.Components),
		Services:           cycloneDXServicesToProto(bom.Services),
		ExternalReferences: cycloneDXExternalReferencesToProto(bom.ExternalReferences),
		Dependencies:       cycloneDXDependenciesToProto(bom.Dependencies),
		Compositions:       cycloneDXCompositionsToProto(bom.Compositions),
		Vulnerabilities:    cycloneDXVulnerabilitiesToProto(bom.Vulnerabilities),
	}
}

func cycloneDXVulnerabilitiesToProto(vulnerabilities *[]cyclonedx.Vulnerability) []*ddCycloneDXProto.Vulnerability {
	return nil
}

func cycloneDXCompositionsToProto(compositions *[]cyclonedx.Composition) []*ddCycloneDXProto.Composition {
	return nil
}

func cycloneDXServicesToProto(services *[]cyclonedx.Service) []*ddCycloneDXProto.Service {
	return nil
}

func cycloneDXComponentTypeToProto(componentType cyclonedx.ComponentType) ddCycloneDXProto.Classification {
	switch componentType {
	case cyclonedx.ComponentTypeApplication:
		return ddCycloneDXProto.Classification_CLASSIFICATION_APPLICATION
	case cyclonedx.ComponentTypeFramework:
		return ddCycloneDXProto.Classification_CLASSIFICATION_FRAMEWORK
	case cyclonedx.ComponentTypeLibrary:
		return ddCycloneDXProto.Classification_CLASSIFICATION_LIBRARY
	case cyclonedx.ComponentTypeOS:
		return ddCycloneDXProto.Classification_CLASSIFICATION_OPERATING_SYSTEM
	case cyclonedx.ComponentTypeDevice:
		return ddCycloneDXProto.Classification_CLASSIFICATION_DEVICE
	case cyclonedx.ComponentTypeFile:
		return ddCycloneDXProto.Classification_CLASSIFICATION_FILE
	case cyclonedx.ComponentTypeContainer:
		return ddCycloneDXProto.Classification_CLASSIFICATION_CONTAINER
	case cyclonedx.ComponentTypeFirmware:
		return ddCycloneDXProto.Classification_CLASSIFICATION_FIRMWARE
	default:
		return ddCycloneDXProto.Classification_CLASSIFICATION_NULL
	}
}

func cycloneDXDependenciesToProto(dependencies *[]cyclonedx.Dependency) []*ddCycloneDXProto.Dependency {
	if dependencies == nil {
		return nil
	}

	dependenciesProto := make([]*ddCycloneDXProto.Dependency, 0, len(*dependencies))
	for _, elem := range *dependencies {
		dependenciesProto = append(dependenciesProto, cycloneDXDependencyToProto(&elem))
	}
	return dependenciesProto
}

func cycloneDXDependencyToProto(elem *cyclonedx.Dependency) *ddCycloneDXProto.Dependency {
	if elem == nil {
		return nil
	}

	return &ddCycloneDXProto.Dependency{
		Ref:          elem.Ref,
		Dependencies: cycloneDXDependenciesToProto(elem.Dependencies),
	}
}

func cycloneDXComponentsToProto(components *[]cyclonedx.Component) []*ddCycloneDXProto.Component {
	if components == nil {
		return nil
	}

	componentsProto := make([]*ddCycloneDXProto.Component, 0, len(*components))
	for _, elem := range *components {
		componentsProto = append(componentsProto, cycloneDXComponentToProto(&elem))
	}
	return componentsProto
}

func cycloneDXComponentToProto(elem *cyclonedx.Component) *ddCycloneDXProto.Component {
	if elem == nil {
		return nil
	}

	return &ddCycloneDXProto.Component{
		BomRef:             elem.BOMRef,
		MimeType:           elem.MIMEType,
		Type:               cycloneDXComponentTypeToProto(elem.Type),
		Name:               elem.Name,
		Version:            elem.Version,
		Purl:               elem.PackageURL,
		Supplier:           cycloneDXOrganizationalEntityToProto(elem.Supplier),
		Author:             elem.Author,
		Publisher:          elem.Publisher,
		Group:              elem.Group,
		Description:        elem.Description,
		Scope:              cycloneDXScopeToProto(elem.Scope),
		Copyright:          elem.Copyright,
		Cpe:                elem.CPE,
		Modified:           boolPointerToProto(elem.Modified),
		Swid:               cycloneDXSWIDToProto(elem.SWID),
		Pedigree:           cycloneDXPedigreeToProto(elem.Pedigree),
		ReleaseNotes:       cycloneDXReleaseNotesToProto(elem.ReleaseNotes),
		Hashes:             cycloneDXHashesToProto(elem.Hashes),
		Licenses:           cycloneDXLicensesToArrayProto(elem.Licenses),
		ExternalReferences: cycloneDXExternalReferencesToProto(elem.ExternalReferences),
		Components:         cycloneDXComponentsToProto(elem.Components),
		Properties:         cycloneDXPropertiesToProto(elem.Properties),
		Evidence:           cycloneDXEvidencesToProto(elem.Evidence),
	}
}

func cycloneDXEvidencesToProto(evidence *cyclonedx.Evidence) []*ddCycloneDXProto.Evidence {
	if evidence == nil {
		return nil
	}
	return []*ddCycloneDXProto.Evidence{
		{
			Licenses:  cycloneDXLicensesToArrayProto(evidence.Licenses),
			Copyright: cycloneDXCopyrightsToProto(evidence.Copyright),
		},
	}
}

func cycloneDXCopyrightsToProto(copyright *[]cyclonedx.Copyright) []*ddCycloneDXProto.EvidenceCopyright {
	if copyright == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.EvidenceCopyright, 0, len(*copyright))
	for _, elem := range *copyright {
		output = append(output, cycloneDXCopyrightToProto(&elem))
	}
	return output
}

func cycloneDXCopyrightToProto(c *cyclonedx.Copyright) *ddCycloneDXProto.EvidenceCopyright {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.EvidenceCopyright{
		Text: c.Text,
	}
}

func cycloneDXReleaseNotesToProto(notes *cyclonedx.ReleaseNotes) *ddCycloneDXProto.ReleaseNotes {
	if notes == nil {
		return nil
	}
	parsedTime, err := time.Parse("2006-01-02T15:04:05+00:00", notes.Timestamp)
	if err != nil {
		seclog.Errorf("couldn't parse the exact timestamp, falling back to time.Time{}")
		parsedTime = time.Time{}
	}

	return &ddCycloneDXProto.ReleaseNotes{
		Type:          notes.Type,
		Title:         notes.Title,
		FeaturedImage: notes.FeaturedImage,
		SocialImage:   notes.SocialImage,
		Description:   notes.Description,
		Timestamp: &timestamp.Timestamp{
			Nanos: int32(parsedTime.Nanosecond()),
		},
		Aliases:    stringArrayPointerToProto(notes.Aliases),
		Tags:       stringArrayPointerToProto(notes.Tags),
		Resolves:   cycloneDXResolvesToProto(notes.Resolves),
		Notes:      cycloneDXNotesToProto(notes.Notes),
		Properties: cycloneDXPropertiesToProto(notes.Properties),
	}
}

func cycloneDXNotesToProto(notes *[]cyclonedx.Note) []*ddCycloneDXProto.Note {
	if notes == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Note, 0, len(*notes))
	for _, elem := range *notes {
		output = append(output, cycloneDXNoteToProto(&elem))
	}
	return output
}

func cycloneDXNoteToProto(c *cyclonedx.Note) *ddCycloneDXProto.Note {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.Note{
		Locale: c.Locale,
		Text:   cycloneDXAttachedTextToProto(&c.Text),
	}
}

func cycloneDXPedigreeToProto(pedigree *cyclonedx.Pedigree) *ddCycloneDXProto.Pedigree {
	if pedigree == nil {
		return nil
	}

	return &ddCycloneDXProto.Pedigree{
		Ancestors:   cycloneDXComponentsToProto(pedigree.Ancestors),
		Descendants: cycloneDXComponentsToProto(pedigree.Descendants),
		Variants:    cycloneDXComponentsToProto(pedigree.Variants),
		Commits:     cycloneDXCommitsToProto(pedigree.Commits),
		Patches:     cycloneDXPatchesToProto(pedigree.Patches),
		Notes:       pedigree.Notes,
	}
}

func cycloneDXPatchesToProto(patches *[]cyclonedx.Patch) []*ddCycloneDXProto.Patch {
	if patches == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Patch, 0, len(*patches))
	for _, elem := range *patches {
		output = append(output, cycloneDXPatchToProto(&elem))
	}
	return output
}

func cycloneDXPatchToProto(c *cyclonedx.Patch) *ddCycloneDXProto.Patch {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.Patch{
		Type:     cycloneDXPatchTypeToProto(c.Type),
		Diff:     cycloneDXDiffToProto(c.Diff),
		Resolves: cycloneDXResolvesToProto(c.Resolves),
	}
}

func cycloneDXResolvesToProto(resolves *[]cyclonedx.Issue) []*ddCycloneDXProto.Issue {
	if resolves == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Issue, 0, len(*resolves))
	for _, elem := range *resolves {
		output = append(output, cycloneDXIssuesToProto(&elem))
	}
	return output
}

func cycloneDXIssuesToProto(c *cyclonedx.Issue) *ddCycloneDXProto.Issue {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.Issue{
		Type:        cycloneDXIssueTypeToProto(c.Type),
		Id:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Source:      cycloneDXSourceToProto(c.Source),
		References:  stringArrayPointerToProto(c.References),
	}
}

func cycloneDXSourceToProto(source *cyclonedx.Source) *ddCycloneDXProto.Source {
	if source == nil {
		return nil
	}

	return &ddCycloneDXProto.Source{
		Name: source.Name,
		Url:  source.URL,
	}
}

func cycloneDXIssueTypeToProto(issueType cyclonedx.IssueType) ddCycloneDXProto.IssueClassification {
	switch issueType {
	case cyclonedx.IssueTypeDefect:
		return ddCycloneDXProto.IssueClassification_ISSUE_CLASSIFICATION_DEFECT
	case cyclonedx.IssueTypeEnhancement:
		return ddCycloneDXProto.IssueClassification_ISSUE_CLASSIFICATION_ENHANCEMENT
	case cyclonedx.IssueTypeSecurity:
		return ddCycloneDXProto.IssueClassification_ISSUE_CLASSIFICATION_SECURITY
	default:
		return ddCycloneDXProto.IssueClassification_ISSUE_CLASSIFICATION_NULL
	}
}

func cycloneDXDiffToProto(diff *cyclonedx.Diff) *ddCycloneDXProto.Diff {
	if diff == nil {
		return nil
	}

	return &ddCycloneDXProto.Diff{
		Text: cycloneDXAttachedTextToProto(diff.Text),
		Url:  diff.URL,
	}
}

func cycloneDXPatchTypeToProto(patchType cyclonedx.PatchType) ddCycloneDXProto.PatchClassification {
	switch patchType {
	case cyclonedx.PatchTypeBackport:
		return ddCycloneDXProto.PatchClassification_PATCH_CLASSIFICATION_BACKPORT
	case cyclonedx.PatchTypeCherryPick:
		return ddCycloneDXProto.PatchClassification_PATCH_CLASSIFICATION_CHERRY_PICK
	case cyclonedx.PatchTypeMonkey:
		return ddCycloneDXProto.PatchClassification_PATCH_CLASSIFICATION_MONKEY
	case cyclonedx.PatchTypeUnofficial:
		return ddCycloneDXProto.PatchClassification_PATCH_CLASSIFICATION_UNOFFICIAL
	default:
		return ddCycloneDXProto.PatchClassification_PATCH_CLASSIFICATION_NULL

	}
}

func cycloneDXCommitsToProto(commits *[]cyclonedx.Commit) []*ddCycloneDXProto.Commit {
	if commits == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Commit, 0, len(*commits))
	for _, elem := range *commits {
		output = append(output, cycloneDXCommitToProto(&elem))
	}
	return output
}

func cycloneDXCommitToProto(c *cyclonedx.Commit) *ddCycloneDXProto.Commit {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.Commit{
		Uid:       c.UID,
		Url:       c.URL,
		Author:    cycloneDXIdentifiableActionToProto(c.Author),
		Committer: cycloneDXIdentifiableActionToProto(c.Committer),
		Message:   c.Message,
	}
}

func cycloneDXIdentifiableActionToProto(author *cyclonedx.IdentifiableAction) *ddCycloneDXProto.IdentifiableAction {
	if author == nil {
		return nil
	}

	parsedTime, err := time.Parse("2006-01-02T15:04:05+00:00", author.Timestamp)
	if err != nil {
		seclog.Errorf("couldn't parse the exact timestamp, falling back to time.Time{}")
		parsedTime = time.Time{}
	}

	return &ddCycloneDXProto.IdentifiableAction{
		Timestamp: &timestamp.Timestamp{
			Nanos: int32(parsedTime.Nanosecond()),
		},
		Name:  author.Name,
		Email: author.Email,
	}
}

func cycloneDXSWIDToProto(swid *cyclonedx.SWID) *ddCycloneDXProto.Swid {
	if swid == nil {
		return nil
	}

	return &ddCycloneDXProto.Swid{
		TagId:      swid.TagID,
		Name:       swid.Name,
		Version:    swid.Version,
		TagVersion: intPointerToInt32(swid.TagVersion),
		Patch:      boolPointerToProto(swid.Patch),
		Text:       cycloneDXAttachedTextToProto(swid.Text),
		Url:        swid.URL,
	}
}

func cycloneDXAttachedTextToProto(text *cyclonedx.AttachedText) *ddCycloneDXProto.AttachedText {
	if text == nil {
		return nil
	}

	return &ddCycloneDXProto.AttachedText{
		Value:       text.Content,
		ContentType: text.ContentType,
		Encoding:    text.Encoding,
	}
}

func intPointerToInt32(elem *int) int32 {
	if elem == nil {
		return 0
	}
	return int32(*elem)
}

func boolPointerToProto(elem *bool) bool {
	if elem == nil {
		return false
	}
	return *elem
}

func cycloneDXScopeToProto(elem cyclonedx.Scope) ddCycloneDXProto.Scope {
	switch elem {
	case cyclonedx.ScopeExcluded:
		return ddCycloneDXProto.Scope_SCOPE_REQUIRED
	case cyclonedx.ScopeOptional:
		return ddCycloneDXProto.Scope_SCOPE_OPTIONAL
	case cyclonedx.ScopeRequired:
		return ddCycloneDXProto.Scope_SCOPE_EXCLUDED
	default:
		return ddCycloneDXProto.Scope_SCOPE_UNSPECIFIED
	}
}

func cycloneDXOrganizationalEntityToProto(elem *cyclonedx.OrganizationalEntity) *ddCycloneDXProto.OrganizationalEntity {
	if elem == nil {
		return nil
	}

	return &ddCycloneDXProto.OrganizationalEntity{
		Name:    elem.Name,
		Url:     stringArrayPointerToProto(elem.URL),
		Contact: cycloneDXOrganizationalContactsToProto(elem.Contact),
	}
}

func stringArrayPointerToProto(s *[]string) []string {
	if s == nil {
		return nil
	}
	return *s
}

func cycloneDXMetadataToProto(elem *cyclonedx.Metadata) *ddCycloneDXProto.Metadata {
	if elem == nil {
		return nil
	}
	parsedTime, err := time.Parse("2006-01-02T15:04:05+00:00", elem.Timestamp)
	if err != nil {
		seclog.Errorf("couldn't parse the exact timestamp, falling back to time.Now()")
		parsedTime = time.Now()
	}

	return &ddCycloneDXProto.Metadata{
		Timestamp: &timestamp.Timestamp{
			Seconds: parsedTime.Unix(),
		},
		Tools:       cycloneDXToolsToProto(elem.Tools),
		Authors:     cycloneDXOrganizationalContactsToProto(elem.Authors),
		Component:   cycloneDXComponentToProto(elem.Component),
		Manufacture: cycloneDXOrganizationalEntityToProto(elem.Manufacture),
		Supplier:    cycloneDXOrganizationalEntityToProto(elem.Supplier),
		Licenses:    cycloneDXLicensesToProto(elem.Licenses),
		Properties:  cycloneDXPropertiesToProto(elem.Properties),
	}
}

func cycloneDXPropertiesToProto(properties *[]cyclonedx.Property) []*ddCycloneDXProto.Property {
	if properties == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Property, 0, len(*properties))
	for _, elem := range *properties {
		output = append(output, cycloneDXPropertyToProto(&elem))
	}
	return output
}

func cycloneDXPropertyToProto(c *cyclonedx.Property) *ddCycloneDXProto.Property {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.Property{
		Name:  c.Name,
		Value: c.Value,
	}
}

func cycloneDXLicensesToProto(licenses *cyclonedx.Licenses) *ddCycloneDXProto.LicenseChoice {
	if licenses == nil {
		return nil
	}
	license := (*licenses)[0]
	return cycloneDXLicenseToProto(&license)
}

func cycloneDXLicensesToArrayProto(licenses *cyclonedx.Licenses) []*ddCycloneDXProto.LicenseChoice {
	if licenses == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.LicenseChoice, 0, len(*licenses))
	for _, elem := range *licenses {
		output = append(output, cycloneDXLicenseToProto(&elem))
	}
	return output
}

func cycloneDXLicenseToProto(l *cyclonedx.LicenseChoice) *ddCycloneDXProto.LicenseChoice {
	if l == nil {
		return nil
	}

	if l.License != nil {
		if len(l.License.ID) != 0 {
			return &ddCycloneDXProto.LicenseChoice{
				Choice: &ddCycloneDXProto.LicenseChoice_License{
					License: &ddCycloneDXProto.License{
						License: &ddCycloneDXProto.License_Id{
							Id: l.License.ID,
						},
						Text: cycloneDXAttachedTextToProto(l.License.Text),
						Url:  l.License.URL,
					},
				},
			}
		}
		return &ddCycloneDXProto.LicenseChoice{
			Choice: &ddCycloneDXProto.LicenseChoice_License{
				License: &ddCycloneDXProto.License{
					License: &ddCycloneDXProto.License_Name{
						Name: l.License.Name,
					},
					Text: cycloneDXAttachedTextToProto(l.License.Text),
					Url:  l.License.URL,
				},
			},
		}
	}
	return &ddCycloneDXProto.LicenseChoice{
		Choice: &ddCycloneDXProto.LicenseChoice_Expression{
			Expression: l.Expression,
		},
	}
}

func cycloneDXOrganizationalContactsToProto(authors *[]cyclonedx.OrganizationalContact) []*ddCycloneDXProto.OrganizationalContact {
	if authors == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.OrganizationalContact, 0, len(*authors))
	for _, elem := range *authors {
		output = append(output, cycloneDXOrganizationalContactToProto(&elem))
	}
	return output
}

func cycloneDXOrganizationalContactToProto(c *cyclonedx.OrganizationalContact) *ddCycloneDXProto.OrganizationalContact {
	return &ddCycloneDXProto.OrganizationalContact{
		Name:  c.Name,
		Email: c.Email,
		Phone: c.Phone,
	}
}

func cycloneDXToolsToProto(elemArray *[]cyclonedx.Tool) []*ddCycloneDXProto.Tool {
	if elemArray == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Tool, 0, len(*elemArray))
	for _, elem := range *elemArray {
		output = append(output, cycloneDXToolToProto(&elem))
	}
	return output
}

func cycloneDXToolToProto(elem *cyclonedx.Tool) *ddCycloneDXProto.Tool {
	if elem == nil {
		return nil
	}

	return &ddCycloneDXProto.Tool{
		Vendor:             elem.Vendor,
		Name:               elem.Name,
		Version:            elem.Version,
		Hashes:             cycloneDXHashesToProto(elem.Hashes),
		ExternalReferences: cycloneDXExternalReferencesToProto(elem.ExternalReferences),
	}
}

func cycloneDXExternalReferencesToProto(references *[]cyclonedx.ExternalReference) []*ddCycloneDXProto.ExternalReference {
	if references == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.ExternalReference, 0, len(*references))
	for _, elem := range *references {
		output = append(output, cycloneDXExternalReferenceToProto(&elem))
	}
	return output
}

func cycloneDXExternalReferenceToProto(c *cyclonedx.ExternalReference) *ddCycloneDXProto.ExternalReference {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.ExternalReference{
		Type:    cycloneDXExternalReferenceTypeToProto(c.Type),
		Url:     c.URL,
		Comment: c.Comment,
		Hashes:  cycloneDXHashesToProto(c.Hashes),
	}
}

func cycloneDXExternalReferenceTypeToProto(referenceType cyclonedx.ExternalReferenceType) ddCycloneDXProto.ExternalReferenceType {
	switch referenceType {
	case cyclonedx.ERTypeAdvisories:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_ADVISORIES
	case cyclonedx.ERTypeBOM:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_BOM
	case cyclonedx.ERTypeBuildMeta:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_BUILD_META
	case cyclonedx.ERTypeBuildSystem:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_BUILD_SYSTEM
	case cyclonedx.ERTypeChat:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_CHAT
	case cyclonedx.ERTypeDistribution:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_DISTRIBUTION
	case cyclonedx.ERTypeDocumentation:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_DOCUMENTATION
	case cyclonedx.ERTypeLicense:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_LICENSE
	case cyclonedx.ERTypeMailingList:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_MAILING_LIST
	case cyclonedx.ERTypeOther:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_OTHER
	case cyclonedx.ERTypeIssueTracker:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_ISSUE_TRACKER
	case cyclonedx.ERTypeReleaseNotes:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_OTHER
	case cyclonedx.ERTypeSocial:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_SOCIAL
	case cyclonedx.ERTypeSupport:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_SUPPORT
	case cyclonedx.ERTypeVCS:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_VCS
	case cyclonedx.ERTypeWebsite:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_WEBSITE
	default:
		return ddCycloneDXProto.ExternalReferenceType_EXTERNAL_REFERENCE_TYPE_OTHER
	}
}

func cycloneDXHashesToProto(hashes *[]cyclonedx.Hash) []*ddCycloneDXProto.Hash {
	if hashes == nil {
		return nil
	}

	output := make([]*ddCycloneDXProto.Hash, 0, len(*hashes))
	for _, elem := range *hashes {
		output = append(output, cycloneDXHashToProto(&elem))
	}
	return output
}

func cycloneDXHashToProto(c *cyclonedx.Hash) *ddCycloneDXProto.Hash {
	if c == nil {
		return nil
	}

	return &ddCycloneDXProto.Hash{
		Value: c.Value,
		Alg:   cycloneDXHashAlgorithmToProto(c.Algorithm),
	}
}

func cycloneDXHashAlgorithmToProto(algorithm cyclonedx.HashAlgorithm) ddCycloneDXProto.HashAlg {
	switch algorithm {
	case cyclonedx.HashAlgoMD5:
		return ddCycloneDXProto.HashAlg_HASH_ALG_MD_5
	case cyclonedx.HashAlgoSHA1:
		return ddCycloneDXProto.HashAlg_HASH_ALG_SHA_1
	case cyclonedx.HashAlgoSHA256:
		return ddCycloneDXProto.HashAlg_HASH_ALG_SHA_256
	case cyclonedx.HashAlgoSHA384:
		return ddCycloneDXProto.HashAlg_HASH_ALG_SHA_384
	case cyclonedx.HashAlgoSHA512:
		return ddCycloneDXProto.HashAlg_HASH_ALG_SHA_512
	case cyclonedx.HashAlgoSHA3_256:
		return ddCycloneDXProto.HashAlg_HASH_ALG_SHA_3_256
	case cyclonedx.HashAlgoSHA3_512:
		return ddCycloneDXProto.HashAlg_HASH_ALG_SHA_3_512
	case cyclonedx.HashAlgoBlake2b_256:
		return ddCycloneDXProto.HashAlg_HASH_ALG_BLAKE_2_B_256
	case cyclonedx.HashAlgoBlake2b_384:
		return ddCycloneDXProto.HashAlg_HASH_ALG_BLAKE_2_B_384
	case cyclonedx.HashAlgoBlake2b_512:
		return ddCycloneDXProto.HashAlg_HASH_ALG_BLAKE_2_B_512
	case cyclonedx.HashAlgoBlake3:
		return ddCycloneDXProto.HashAlg_HASH_ALG_BLAKE_3
	default:
		return ddCycloneDXProto.HashAlg_HASH_ALG_NULL
	}
}
