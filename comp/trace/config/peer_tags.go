package config

import (
	_ "embed"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/ini.v1"
)

//go:embed peer_tags.ini
var peerTagFile []byte

var defaultPeerTags = func() []string {
	var tags []string = []string{"_dd.base_service"}

	cfg, err := ini.Load(peerTagFile)
	if err != nil {
		log.Error("Error loading file for peer tags: ", err)
		return tags
	}
	keys := cfg.Section("dd.apm.peer.tags").Keys()

	for _, key := range keys {
		value := strings.Split(key.Value(), ",")
		tags = append(tags, value...)
	}

	sort.Strings(tags)

	return tags
}()

func preparePeerTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	var deduped []string
	seen := make(map[string]struct{})
	for _, t := range tags {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			deduped = append(deduped, t)
		}
	}
	sort.Strings(deduped)
	return deduped
}
