// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	dockerUtil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// configPath refers to the configuration that can be passed over a docker label,
// this feature is commonly named 'ad' or 'autodiscovery'.
const configPath = "com.datadoghq.ad.logs"

// Container represents a container to tail logs from.
type Container struct {
	container     types.Container
	service       *service.Service
	shortLived    bool
	discoveryTime time.Time
}

// NewContainer returns a new Container
func NewContainer(container types.Container, service *service.Service) *Container {
	return &Container{
		container:     container,
		service:       service,
		discoveryTime: time.Now(),
	}
}

// findSource returns the source that most likely matches the container,
// if no source is found return nil
func (c *Container) FindSource(sources []*config.LogSource) *config.LogSource {
	var bestMatch *config.LogSource
	for _, source := range sources {
		if source.Config.Identifier != "" && c.isIdentifierMatch(source.Config.Identifier) {
			// perfect match between the source and the container
			return source
		}
		if !c.IsMatch(source) {
			continue
		}
		if bestMatch == nil {
			bestMatch = source
		}
		if c.computeScore(bestMatch) < c.computeScore(source) {
			bestMatch = source
		}
	}
	return bestMatch
}

// SetShortLived changes the short-lived state of the container.
func (c *Container) SetShortLived(shortLived bool) {
	c.shortLived = shortLived
}

// IsShortLived returns true if the container is short lived.
func (c *Container) IsShortLived() bool {
	return c.shortLived
}

// DiscoveryTime returns the discovery time of the container.
func (c *Container) DiscoveryTime() time.Time {
	return c.discoveryTime
}

// getShortImageName resolves the short image name of a container by calling the docker daemon
// This call is blocking
func (c *Container) getShortImageName() (string, error) {
	var (
		err       error
		shortName string
	)

	du, err := dockerUtil.GetDockerUtil()
	if err != nil {
		log.Debugf("Cannot get DockerUtil: %v", err)
		return shortName, err
	}
	imageName := c.container.ImageID
	imageName, err = du.ResolveImageName(imageName)
	if err != nil {
		log.Debugf("Could not resolve image name %s: %s", imageName, err)
		return shortName, err
	}
	_, shortName, _, err = containers.SplitImageName(imageName)
	if err != nil {
		log.Debugf("Cannot parse image name: %v", err)
	}
	return shortName, err
}

// computeScore returns the matching score between the container and the source.
func (c *Container) computeScore(source *config.LogSource) int {
	score := 0
	if c.isImageMatch(source.Config.Image) {
		score++
	}
	if c.isLabelMatch(source.Config.Label) {
		score++
	}
	if c.isNameMatch(source.Config.Name) {
		score++
	}
	return score
}

// IsMatch returns true if the source matches with the container.
func (c *Container) IsMatch(source *config.LogSource) bool {
	if (source.Config.Identifier != "" || c.ContainsADIdentifier()) && !c.isIdentifierMatch(source.Config.Identifier) {
		// there is only one source matching a container when it contains an autodiscovery identifier,
		// the one which has an identifier equals to the container identifier.
		return false
	}
	if source.Config.Image != "" && !c.isImageMatch(source.Config.Image) {
		return false
	}
	if source.Config.Label != "" && !c.isLabelMatch(source.Config.Label) {
		return false
	}
	if source.Config.Name != "" && !c.isNameMatch(source.Config.Name) {
		return false
	}
	return true
}

// isIdentifierMatch returns if identifier matches with container identifier.
func (c *Container) isIdentifierMatch(identifier string) bool {
	return c.container.ID == identifier
}

// digestPrefix represents a prefix that can be added to an image name.
const digestPrefix = "@sha256:"

// tagSeparator represents the separator in between an image name and its tag.
const tagSeparator = ":"

// isImageMatch returns true if the image of the container matches with imageFilter.
// The image of a container can have the following formats:
// - '[<repository>/]image[:<tag>]',
// - '[<repository>/]image[@sha256:<digest>]'
// The imageFilter must respect the format '[<repository>/]image[:<tag>]'.
func (c *Container) isImageMatch(imageFilter string) bool {
	// Trim digest if present
	split := strings.SplitN(c.container.Image, digestPrefix, 2)
	image := split[0]
	if !strings.Contains(imageFilter, tagSeparator) {
		// trim tag if present
		split := strings.SplitN(image, tagSeparator, 2)
		image = split[0]
	}
	// Expect prefix to end with '/'
	repository := strings.TrimSuffix(image, imageFilter)
	return len(repository) == 0 || strings.HasSuffix(repository, "/")
}

// isNameMatch returns true if one of the container name matches with the filter.
func (c *Container) isNameMatch(nameFilter string) bool {
	re, err := regexp.Compile(nameFilter)
	if err != nil {
		log.Warn("used invalid name to filter containers: ", nameFilter)
		return false
	}
	for _, name := range c.container.Names {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// isLabelMatch returns true if container labels contains at least one label from labelFilter.
func (c *Container) isLabelMatch(labelFilter string) bool {
	// Expect a comma-separated list of labels, eg: foo:bar, baz
	for _, value := range strings.Split(labelFilter, ",") {
		// Trim whitespace, then check whether the label format is either key:value or key=value
		label := strings.TrimSpace(value)
		parts := strings.FieldsFunc(label, func(c rune) bool {
			return c == ':' || c == '='
		})
		// If we have exactly two parts, check there is a container label that matches both.
		// Otherwise fall back to checking the whole label exists as a key.
		if _, exists := c.container.Labels[label]; exists || len(parts) == 2 && c.container.Labels[parts[0]] == parts[1] {
			return true
		}
	}
	return false
}

// ContainsADIdentifier returns true if the container contains an autodiscovery identifier.
func (c *Container) ContainsADIdentifier() bool {
	return ContainsADIdentifier(c)
}
