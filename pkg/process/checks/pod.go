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

	yaml "gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
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
func (c *PodCheck) Endpoint() string { return "/api/v1/orchestrator" }

// RealTime indicates if this check only runs in real-time mode.
func (c *PodCheck) RealTime() bool { return false }

// Run runs the PodCheck to collect a list of running pods
func (c *PodCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	c.Lock()
	defer c.Unlock()

	util, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	podList, err := util.GetRawLocalPodList()
	if err != nil {
		return nil, err
	}

	log.Debugf("Collected %d pods", len(podList))

	podMsgs := []*model.Pod{}

	for p := 0; p < len(podList); p++ {
		for c := 0; c < len(podList[p].Spec.Containers); c++ {
			scrubContainer(&podList[p].Spec.Containers[c], cfg)
		}
		for c := 0; c < len(podList[p].Spec.InitContainers); c++ {
			scrubContainer(&podList[p].Spec.Containers[c], cfg)
		}
		tags, err := tagger.Tag(kubelet.PodUIDToTaggerEntityName(string(podList[p].UID)), collectors.HighCardinality)
		if err != nil {
			log.Warnf("Could not retrieve tags for pod: %s", err)
			continue
		}
		yamlPod, _ := yaml.Marshal(podList[p])
		log.Info(string(yamlPod))
		// TODO: add more fields
		podModel := model.Pod{
			Name: podList[p].Name,
			Tags: tags,
			Yaml: yamlPod,
		}

		podMsgs = append(podMsgs, &podModel)
	}

	message := model.CollectorPod{
		HostName: cfg.HostName,
		Pods:     podMsgs,
	}

	messages := []model.MessageBody{
		&message,
	}

	return messages, nil
}

func scrubContainer(c *v1.Container, cfg *config.AgentConfig) {
	// scrub command line
	scrubbedCmd, _ := cfg.Scrubber.ScrubCommand(c.Command)
	c.Command = scrubbedCmd
	// scrub env vars
	for e := 0; e < len(c.Env); e++ {
		scrubbedVal, err := log.CredentialsCleanerBytes([]byte(c.Env[e].Value))
		if err == nil {
			c.Env[e].Value = string(scrubbedVal)
		}
	}
}
