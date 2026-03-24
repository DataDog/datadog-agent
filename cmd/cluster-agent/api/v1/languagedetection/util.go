// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

// containersLanguageWithDirtyFlag encapsulates containers languages along with a dirty flag
// The dirty flag is used to know if the containers languages are flushed to workload metadata store or not.
// The dirty flag is reset when languages are flushed to workload metadata store.
type containersLanguageWithDirtyFlag struct {
	languages languagemodels.TimedContainersLanguages
	dirty     bool
}

func newContainersLanguageWithDirtyFlag() *containersLanguageWithDirtyFlag {
	return &containersLanguageWithDirtyFlag{
		languages: make(languagemodels.TimedContainersLanguages),
		dirty:     true,
	}
}

////////////////////////////////
//                            //
//      Owners Languages      //
//                            //
////////////////////////////////

// OwnersLanguages maps a namespaced owner (kubernetes resource) to containers languages
// This is mainly used as a preliminary storage for detected languages of kubernetes resources prior to storing
// languages in workload meta store.
//
// It is needed in order to:
//   - control what to store in workload metadata store based on detected languages TTL and last detection time
//   - avoid flakiness in the set of detected languages during the rollout of a kubernetes resource;
//     during rollout the handler may, depending on the deployment size for example, receive different languages
//     based on whether the source pod has been rolled out yet or not, which can cause flakiness in the set of detected languages.
//
// Components using OwnersLanguages should only invoke the thread-safe methods: mergeAndFlush, cleanExpiredLanguages,
// and cleanRemovedOwners.
// Other methods are not thread-safe; they are supposed to be invoked only within mergeAndFlush.
type OwnersLanguages struct {
	containersLanguages map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag
	mutex               sync.Mutex
}

func newOwnersLanguages() *OwnersLanguages {
	return &OwnersLanguages{
		containersLanguages: make(map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag),
		mutex:               sync.Mutex{},
	}
}

// String returns the current state of the owners languages including the detected languages
// and whether they are dirty or not.
// This method is thread-safe.
func (ownersLanguages *OwnersLanguages) String() string {
	if ownersLanguages == nil {
		return ""
	}

	ownersLanguages.mutex.Lock()
	defer ownersLanguages.mutex.Unlock()

	var sb strings.Builder
	sb.WriteString("[")

	firstOwner := true
	for owner, langs := range ownersLanguages.containersLanguages {
		if !firstOwner {
			sb.WriteString(",")
		}

		sb.WriteString(fmt.Sprintf("(%s/%s/%s, %v,[", owner.Namespace, owner.Kind, owner.Name, langs.dirty))

		firstContainer := true
		for container, languageSet := range langs.languages {
			if !firstContainer {
				sb.WriteString(",")
			}
			sb.WriteString(container.Name + ": (")

			languages := make([]string, 0, len(languageSet))
			for languagename := range languageSet {
				languages = append(languages, string(languagename))
			}
			sb.WriteString(strings.Join(languages, ","))

			sb.WriteString(")")
			firstContainer = false
		}

		sb.WriteString("])")
		firstOwner = false
	}

	sb.WriteString("]")

	return sb.String()
}

// getOrInitialize returns the containers languages for a specific namespaced owner, initialising it if it doesn't already
// exist.
// This method is not thread-safe.
func (ownersLanguages *OwnersLanguages) getOrInitialize(reference langUtil.NamespacedOwnerReference) *containersLanguageWithDirtyFlag {
	_, found := ownersLanguages.containersLanguages[reference]
	if !found {
		ownersLanguages.containersLanguages[reference] = newContainersLanguageWithDirtyFlag()
	}
	containersLanguages := ownersLanguages.containersLanguages[reference]
	return containersLanguages
}

// merge merges another owners languages instance data with the current containers languages.
// This method is not thread-safe.
func (ownersLanguages *OwnersLanguages) merge(other *OwnersLanguages) {
	for owner, containersLanguages := range other.containersLanguages {
		langsWithDirtyFlag := ownersLanguages.getOrInitialize(owner)
		if modified := langsWithDirtyFlag.languages.Merge(containersLanguages.languages); modified {
			langsWithDirtyFlag.dirty = true
		}
	}
}

// flush flushes to workloadmeta store containers languages that have dirty flag set to true, and then resets
// dirty flag to false.
// This method acquires the mutex, collects dirty events and marks them as not dirty, releases the mutex,
// then pushes all events in a single batch.
func (ownersLanguages *OwnersLanguages) flush(wlm workloadmeta.Component) error {
	ownersLanguages.mutex.Lock()
	var events []workloadmeta.Event
	var pushErrors []error
	for owner, containersLanguages := range ownersLanguages.containersLanguages {
		if !containersLanguages.dirty {
			continue
		}

		if event := generatePushEvent(owner, containersLanguages.languages); event != nil {
			events = append(events, *event)
			containersLanguages.dirty = false
		} else {
			pushErrors = append(
				pushErrors,
				fmt.Errorf(
					"failed to generate push event for %v %v/%v. reason: unsupported resource kind",
					owner.Kind,
					owner.Namespace,
					owner.Name),
			)
		}
	}
	ownersLanguages.mutex.Unlock()

	if len(events) > 0 {
		// Push() error is ignored because it only returns an error for invalid event types, and generatePushEvent()
		// always creates events with valid types (EventTypeSet or EventTypeUnset).
		// Push staying outsides the mutex prevents deadlock when Push() blocks on workloadmeta's eventCh.
		_ = wlm.Push(workloadmeta.SourceLanguageDetectionServer, events...)
	}
	return errors.Join(pushErrors...)
}

// mergeAndFlush merges the current containers languages for all owners with owners containers languages
// passed as an argument. It then flushes the containers languages having a set dirty flag to workloadmeta store
// and resets dirty flags to false.
// This method is thread-safe, and it serves as the unique entrypoint to instances of this type.
func (ownersLanguages *OwnersLanguages) mergeAndFlush(other *OwnersLanguages, wlm workloadmeta.Component) error {
	ownersLanguages.mutex.Lock()
	ownersLanguages.merge(other)
	ownersLanguages.mutex.Unlock()

	return ownersLanguages.flush(wlm)
}

// clean removes any expired language and flushes data to workloadmeta store
// This method is thread-safe.
func (ownersLanguages *OwnersLanguages) cleanExpiredLanguages(wlm workloadmeta.Component) {
	ownersLanguages.mutex.Lock()
	for _, containersLanguages := range ownersLanguages.containersLanguages {
		if containersLanguages.languages.RemoveExpiredLanguages() {
			containersLanguages.dirty = true
		}
	}
	ownersLanguages.mutex.Unlock()

	_ = ownersLanguages.flush(wlm)
}

// syncFromInjectableLanguages updates DetectedLanguages to match InjectableLanguages for followers
// This ensures consistency when a follower becomes leader
// This method is thread-safe.
func (ownersLanguages *OwnersLanguages) syncFromInjectableLanguages(owner langUtil.NamespacedOwnerReference, injectableLanguages languagemodels.ContainersLanguages, ttl time.Duration) {
	ownersLanguages.mutex.Lock()
	// Update the in-memory state to match injectable languages
	langsWithDirtyFlag := ownersLanguages.getOrInitialize(owner)

	// Convert injectable languages to timed containers languages
	// Use the configured TTL for consistency with leader behavior
	timedLangs := make(languagemodels.TimedContainersLanguages)
	expiration := time.Now().Add(ttl)

	for container, langSet := range injectableLanguages {
		timedLangs[container] = make(languagemodels.TimedLanguageSet)
		for lang := range langSet {
			timedLangs[container][lang] = expiration
		}
	}

	// Replace the languages entirely (not merge) to handle deprecations
	// This ensures DetectedLangs exactly matches InjectableLangs
	langsWithDirtyFlag.languages = timedLangs
	langsWithDirtyFlag.dirty = true
	ownersLanguages.mutex.Unlock()
}

// handleKubeAPIServerUnsetEvents handles unset events emitted by the kubeapiserver
// events with type EventTypeSet are skipped
// events with type EventTypeUnset are handled by deleting the corresponding owner from OwnersLanguages
// and by pushing a new event to workloadmeta that unsets detected languages data for the concerned kubernetes resource
// This method is thread-safe.
func (ownersLanguages *OwnersLanguages) handleKubeAPIServerUnsetEvents(events []workloadmeta.Event, wlm workloadmeta.Component) {
	var unsetEvents []workloadmeta.Event

	ownersLanguages.mutex.Lock()
	for _, event := range events {
		kind := event.Entity.GetID().Kind

		if event.Type != workloadmeta.EventTypeUnset {
			// only unset events should be handled
			continue
		}

		switch kind {
		case workloadmeta.KindKubernetesDeployment:
			// extract deployment name and namespace from entity id
			deployment := event.Entity.(*workloadmeta.KubernetesDeployment)
			deploymentIDs := strings.Split(deployment.GetID().ID, "/")
			namespace := deploymentIDs[0]
			deploymentName := deploymentIDs[1]
			delete(ownersLanguages.containersLanguages, langUtil.NewNamespacedOwnerReference("apps/v1", langUtil.KindDeployment, deploymentName, namespace))
			unsetEvents = append(unsetEvents, workloadmeta.Event{
				Type:   workloadmeta.EventTypeUnset,
				Entity: deployment,
			})
		}
	}
	ownersLanguages.mutex.Unlock()

	if len(unsetEvents) > 0 {
		_ = wlm.Push(workloadmeta.SourceLanguageDetectionServer, unsetEvents...)
	}
}

// cleanRemovedOwners listens to workloadmeta kubeapiserver events and removes
// languages of owners that are deleted.
// It also unsets detected languages in workloadmeta store for deleted owners
// This method is blocking, and should be called within a goroutine
// This method is thread-safe.
func (ownersLanguages *OwnersLanguages) cleanRemovedOwners(wlm workloadmeta.Component) {

	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceKubeAPIServer).
		SetEventType(workloadmeta.EventTypeUnset).
		AddKind(workloadmeta.KindKubernetesDeployment).
		Build()

	evBundle := wlm.Subscribe("language-detection-handler", workloadmeta.NormalPriority, filter)
	defer wlm.Unsubscribe(evBundle)

	for evChan := range evBundle {
		evChan.Acknowledge()
		ownersLanguages.handleKubeAPIServerUnsetEvents(evChan.Events, wlm)
	}
}

////////////////////////////////
//                            //
//           Utils            //
//                            //
////////////////////////////////

// generatePushEvent generates a workloadmeta push event based on the owner languages
// if owner has no detected languages, it generates an unset event
// else it generates a set event
func generatePushEvent(owner langUtil.NamespacedOwnerReference, languages languagemodels.TimedContainersLanguages) *workloadmeta.Event {
	_, found := langUtil.SupportedBaseOwners[owner.Kind]

	if !found {
		return nil
	}

	containerLanguages := make(languagemodels.ContainersLanguages)

	for container, langsetWithExpiration := range languages {
		containerLanguages[container] = make(languagemodels.LanguageSet)
		for lang := range langsetWithExpiration {
			containerLanguages[container][lang] = struct{}{}
		}
	}

	eventType := workloadmeta.EventTypeSet
	if len(containerLanguages) == 0 {
		eventType = workloadmeta.EventTypeUnset
	}

	switch owner.Kind {
	case langUtil.KindDeployment:
		return &workloadmeta.Event{
			Type: eventType,
			Entity: &workloadmeta.KubernetesDeployment{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesDeployment,
					ID:   fmt.Sprintf("%s/%s", owner.Namespace, owner.Name),
				},
				DetectedLanguages: containerLanguages,
			},
		}
	default:
		return nil
	}
}

// getContainersLanguagesFromPodDetail returns containers languages objects for both standard containers
// and for init container
func getContainersLanguagesFromPodDetail(podDetail *pbgo.PodLanguageDetails, expirationTime time.Time) *languagemodels.TimedContainersLanguages {
	containersLanguages := make(languagemodels.TimedContainersLanguages)

	// handle standard containers
	for _, containerLanguageDetails := range podDetail.ContainerDetails {
		containerName := containerLanguageDetails.ContainerName
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containersLanguages.GetOrInitialize(*languagemodels.NewContainer(containerName)).Add(languagemodels.LanguageName(language.Name), expirationTime)
		}
	}

	// handle init containers
	for _, containerLanguageDetails := range podDetail.InitContainerDetails {
		containerName := containerLanguageDetails.ContainerName
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containersLanguages.GetOrInitialize(*languagemodels.NewInitContainer(containerName)).Add(languagemodels.LanguageName(language.Name), expirationTime)
		}
	}

	return &containersLanguages
}

// getOwnersLanguages constructs OwnersLanguages from owners (i.e. k8s parent resource)
func getOwnersLanguages(requestData *pbgo.ParentLanguageAnnotationRequest, expirationTime time.Time) *OwnersLanguages {
	ownersContainersLanguages := newOwnersLanguages()

	podDetails := requestData.PodDetails

	for _, podDetail := range podDetails {
		namespacedOwnerRef := langUtil.GetNamespacedBaseOwnerReference(podDetail)

		if _, found := langUtil.SupportedBaseOwners[namespacedOwnerRef.Kind]; found {
			containersLanguages := *getContainersLanguagesFromPodDetail(podDetail, expirationTime)
			langsWithDirtyFlag := ownersContainersLanguages.getOrInitialize(namespacedOwnerRef)
			if modified := langsWithDirtyFlag.languages.Merge(containersLanguages); modified {
				langsWithDirtyFlag.dirty = true
			}
		}
	}

	return ownersContainersLanguages
}
