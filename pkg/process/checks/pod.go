// +build linux,kubelet

package checks

import (
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Pod is a singleton PodCheck.
var Pod = &PodCheck{}

// PodCheck is a check that returns container metadata and stats.
type PodCheck struct {
	sync.Mutex

	sysInfo                 *model.SystemInfo
	containerFailedLogLimit *util.LogLimit
}

// Init initializes a PodCheck instance.
func (c *PodCheck) Init(cfg *config.AgentConfig, info *model.SystemInfo) {
	c.sysInfo = info
	c.containerFailedLogLimit = util.NewLogLimit(10, time.Minute*10)
}

// Name returns the name of the ProcessCheck.
func (c *PodCheck) Name() string { return "pod" }

// Endpoint returns the endpoint where this check is submitted.
func (c *PodCheck) Endpoint() string { return "/api/v1/pod" }

// RealTime indicates if this check only runs in real-time mode.
func (c *PodCheck) RealTime() bool { return false }

// Run runs the PodCheck to collect a list of running pods
func (c *PodCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	c.Lock()
	defer c.Unlock()

	log.Info("Running pod check")

	util, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	podList, err := util.GetRawLocalPodList()
	if err != nil {
		return nil, err
	}

	log.Debugf("Collected %d pods", len(podList))

	scrubbedPodlist := make([]kubelet.ScrubbedPod, len(podList))

	for p := 0; p < len(podList); p++ {
		for c := 0; c < len(podList[p].Spec.Containers); c++ {
			// scrub command line
			scrubbedCmd, _ := cfg.Scrubber.ScrubCommand(podList[p].Spec.Containers[c].Command)
			podList[p].Spec.Containers[c].Command = scrubbedCmd
			// scrub env vars
			for e := 0; e < len(podList[p].Spec.Containers[c].Env); e++ {
				scrubbedVal, err := log.CredentialsCleanerBytes([]byte(podList[p].Spec.Containers[c].Env[e].Value))
				if err == nil {
					podList[p].Spec.Containers[c].Env[e].Value = string(scrubbedVal)
				}
			}
		}
		for c := 0; c < len(podList[p].Spec.InitContainers); c++ {
			// scrub command line
			scrubbedCmd, _ := cfg.Scrubber.ScrubCommand(podList[p].Spec.Containers[c].Command)
			podList[p].Spec.Containers[c].Command = scrubbedCmd
			// scrub env vars
			for e := 0; e < len(podList[p].Spec.Containers[c].Env); e++ {
				scrubbedVal, err := log.CredentialsCleanerBytes([]byte(podList[p].Spec.Containers[c].Env[e].Value))
				if err == nil {
					podList[p].Spec.Containers[c].Env[e].Value = string(scrubbedVal)
				}
			}
		}
		tags, err := tagger.Tag(kubelet.PodUIDToTaggerEntityName(string(podList[p].UID)), collectors.HighCardinality)
		if err != nil {
			log.Warnf("Could not retrieve tags for pod: %s", err)
			continue
		}
		scrubbedPod := kubelet.ScrubbedPod{
			Pod:  podList[p],
			Tags: tags,
		}

		log.Infof("%v", scrubbedPod)
		scrubbedPodlist = append(scrubbedPodlist, scrubbedPod)
	}

	// TODO: payload

	messages := make([]model.MessageBody, 0)
	return messages, nil
}
