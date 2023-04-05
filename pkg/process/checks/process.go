// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/gopsutil/cpu"
	"go.uber.org/atomic"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/subscriptions"
)

const (
	emptyCtrID                 = ""
	configPrefix               = "process_config."
	configCustomSensitiveWords = configPrefix + "custom_sensitive_words"
	configScrubArgs            = configPrefix + "scrub_args"
	configStripProcArgs        = configPrefix + "strip_proc_arguments"
	configDisallowList         = configPrefix + "blacklist_patterns"
)

// NewProcessCehck returns an instance of the ProcessCheck.
func NewProcessCheck() *ProcessCheck {
	return &ProcessCheck{
		scrubber: procutil.NewDefaultDataScrubber(),
	}
}

var errEmptyCPUTime = errors.New("empty CPU time information returned")

const (
	ProcessDiscoveryHint int32 = 1 << iota // 1
)

// ProcessCheck collects full state, including cmdline args and related metadata,
// for live and running processes. The instance will store some state between
// checks that will be used for rates, cpu calculations, etc.
type ProcessCheck struct {
	probe procutil.Probe
	// scrubber is a DataScrubber to hide command line sensitive words
	scrubber *procutil.DataScrubber

	// disallowList to hide processes
	disallowList []*regexp.Regexp

	hostInfo                   *HostInfo
	lastCPUTime                cpu.TimesStat
	lastProcs                  map[int32]*procutil.Process
	lastRun                    time.Time
	containerProvider          util.ContainerProvider
	lastContainerRates         map[string]*util.ContainerRateMetrics
	realtimeLastContainerRates map[string]*util.ContainerRateMetrics
	networkID                  string

	realtimeLastCPUTime cpu.TimesStat
	realtimeLastProcs   map[int32]*procutil.Stats
	realtimeLastRun     time.Time

	notInitializedLogLimit *util.LogLimit

	// lastPIDs is []int32 that holds PIDs that the check fetched last time,
	// will be reused by RT process collection to get stats
	lastPIDs []int32

	// SysprobeProcessModuleEnabled tells the process check wheither to use the RemoteSystemProbeUtil to gather privileged process stats
	SysprobeProcessModuleEnabled bool

	maxBatchSize  int
	maxBatchBytes int

	checkCount uint32
	skipAmount uint32

	lastConnRates     *atomic.Pointer[ProcessConnRates]
	connRatesReceiver subscriptions.Receiver[ProcessConnRates]
}

// Init initializes the singleton ProcessCheck.
func (p *ProcessCheck) Init(syscfg *SysProbeConfig, info *HostInfo) error {
	p.hostInfo = info
	p.SysprobeProcessModuleEnabled = syscfg.ProcessModuleEnabled
	p.probe = newProcessProbe(procutil.WithPermission(syscfg.ProcessModuleEnabled))
	p.containerProvider = util.GetSharedContainerProvider()

	p.notInitializedLogLimit = util.NewLogLimit(1, time.Minute*10)

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	p.networkID = networkID

	p.maxBatchSize = getMaxBatchSize()
	p.maxBatchBytes = getMaxBatchBytes()

	p.skipAmount = uint32(ddconfig.Datadog.GetInt32("process_config.process_discovery.hint_frequency"))
	if p.skipAmount == 0 {
		log.Warnf("process_config.process_discovery.hint_frequency must be greater than 0. using default value %d",
			ddconfig.DefaultProcessDiscoveryHintFrequency)
		ddconfig.Datadog.Set("process_config.process_discovery.hint_frequency", ddconfig.DefaultProcessDiscoveryHintFrequency)
		p.skipAmount = ddconfig.DefaultProcessDiscoveryHintFrequency
	}

	initScrubber(p.scrubber)

	p.disallowList = initDisallowList()

	p.initConnRates()
	return nil
}

func (p *ProcessCheck) initConnRates() {
	p.lastConnRates = atomic.NewPointer[ProcessConnRates](nil)
	p.connRatesReceiver = subscriptions.NewReceiver[ProcessConnRates]()

	go p.updateConnRates()
}

func (p *ProcessCheck) updateConnRates() {
	for {
		connRates, ok := <-p.connRatesReceiver.Ch
		if !ok {
			return
		}
		p.lastConnRates.Store(&connRates)
	}
}

func (p *ProcessCheck) getLastConnRates() ProcessConnRates {
	if p.lastConnRates == nil {
		return nil
	}
	if result := p.lastConnRates.Load(); result != nil {
		return *result
	}
	return nil
}

// IsEnabled returns true if the check is enabled by configuration
func (p *ProcessCheck) IsEnabled() bool {
	return ddconfig.Datadog.GetBool("process_config.process_collection.enabled")
}

// SupportsRunOptions returns true if the check supports RunOptions
func (p *ProcessCheck) SupportsRunOptions() bool {
	return true
}

// Name returns the name of the ProcessCheck.
func (p *ProcessCheck) Name() string { return ProcessCheckName }

// Realtime indicates if this check only runs in real-time mode.
func (p *ProcessCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (p *ProcessCheck) ShouldSaveLastRun() bool { return true }

// Cleanup frees any resource held by the ProcessCheck before the agent exits
func (p *ProcessCheck) Cleanup() {}

func (p *ProcessCheck) run(groupID int32, collectRealTime bool) (RunResult, error) {
	start := time.Now()
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if len(cpuTimes) == 0 {
		return nil, errEmptyCPUTime
	}

	procs, err := p.probe.ProcessesByPID(time.Now(), true)
	if err != nil {
		return nil, err
	}

	// stores lastPIDs to be used by RTProcess
	p.lastPIDs = p.lastPIDs[:0]
	for pid := range procs {
		p.lastPIDs = append(p.lastPIDs, pid)
	}

	if sysProbeUtil := p.getRemoteSysProbeUtil(); sysProbeUtil != nil {
		mergeProcWithSysprobeStats(p.lastPIDs, procs, sysProbeUtil)
	}

	var containers []*model.Container
	var pidToCid map[int]string
	var lastContainerRates map[string]*util.ContainerRateMetrics
	cacheValidity := cacheValidityNoRT
	if collectRealTime {
		cacheValidity = cacheValidityRT
	}

	containers, lastContainerRates, pidToCid, err = p.containerProvider.GetContainers(cacheValidity, p.lastContainerRates)
	if err == nil {
		p.lastContainerRates = lastContainerRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	// Keep track of containers addresses
	LocalResolver.LoadAddrs(containers, pidToCid)

	// End check early if this is our first run.
	if p.lastProcs == nil {
		p.lastProcs = procs
		p.lastCPUTime = cpuTimes[0]
		p.lastRun = time.Now()

		if collectRealTime {
			p.realtimeLastCPUTime = p.lastCPUTime
			p.realtimeLastProcs = procsToStats(p.lastProcs)
			p.realtimeLastRun = p.lastRun
		}
		return CombinedRunResult{}, nil
	}

	collectorProcHints := p.generateHints()
	p.checkCount++

	connsRates := p.getLastConnRates()
	procsByCtr := fmtProcesses(p.scrubber, p.disallowList, procs, p.lastProcs, pidToCid, cpuTimes[0], p.lastCPUTime, p.lastRun, connsRates)
	messages, totalProcs, totalContainers := createProcCtrMessages(p.hostInfo, procsByCtr, containers, p.maxBatchSize, p.maxBatchBytes, groupID, p.networkID, collectorProcHints)

	// Store the last state for comparison on the next run.
	// Note: not storing the filtered in case there are new processes that haven't had a chance to show up twice.
	p.lastProcs = procs
	p.lastCPUTime = cpuTimes[0]
	p.lastRun = time.Now()

	result := &CombinedRunResult{
		Standard: messages,
	}
	if collectRealTime {
		stats := procsToStats(p.lastProcs)

		if p.realtimeLastProcs != nil {
			// TODO: deduplicate chunking with RT collection
			chunkedStats := fmtProcessStats(p.maxBatchSize, stats, p.realtimeLastProcs, pidToCid, cpuTimes[0], p.realtimeLastCPUTime, p.realtimeLastRun, connsRates)
			groupSize := len(chunkedStats)
			chunkedCtrStats := convertAndChunkContainers(containers, groupSize)

			messages := make([]model.MessageBody, 0, groupSize)
			for i := 0; i < groupSize; i++ {
				messages = append(messages, &model.CollectorRealTime{
					HostName:          p.hostInfo.HostName,
					Stats:             chunkedStats[i],
					ContainerStats:    chunkedCtrStats[i],
					GroupId:           groupID,
					GroupSize:         int32(groupSize),
					NumCpus:           int32(len(p.hostInfo.SystemInfo.Cpus)),
					TotalMemory:       p.hostInfo.SystemInfo.TotalMemory,
					ContainerHostType: p.hostInfo.ContainerHostType,
				})
			}
			result.Realtime = messages
		}

		p.realtimeLastCPUTime = p.lastCPUTime
		p.realtimeLastProcs = stats
		p.realtimeLastRun = p.lastRun
	}

	statsd.Client.Gauge("datadog.process.containers.host_count", float64(totalContainers), []string{}, 1) //nolint:errcheck
	statsd.Client.Gauge("datadog.process.processes.host_count", float64(totalProcs), []string{}, 1)       //nolint:errcheck
	log.Debugf("collected processes in %s", time.Now().Sub(start))

	return result, nil
}

func (p *ProcessCheck) generateHints() int32 {
	var hints int32

	if p.checkCount%p.skipAmount == 0 {
		log.Tracef("generated a process discovery hint on check #%d", p.checkCount)
		hints |= ProcessDiscoveryHint
	}
	return hints
}

func procsToStats(procs map[int32]*procutil.Process) map[int32]*procutil.Stats {
	stats := map[int32]*procutil.Stats{}
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats
}

// Run collects process data (regular metadata + stats) and/or realtime process data (stats only)
func (p *ProcessCheck) Run(nextGroupID func() int32, options *RunOptions) (RunResult, error) {
	if options == nil {
		return p.run(nextGroupID(), false)
	}

	if options.RunStandard {
		log.Tracef("Running process check")
		return p.run(nextGroupID(), options.RunRealtime)
	}

	if options.RunRealtime {
		log.Tracef("Running rtprocess check")
		return p.runRealtime(nextGroupID())
	}
	return nil, errors.New("invalid run options for check")
}

func createProcCtrMessages(
	hostInfo *HostInfo,
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	maxBatchSize int,
	maxBatchWeight int,
	groupID int32,
	networkID string,
	hints int32,
) ([]model.MessageBody, int, int) {
	collectorProcs, totalProcs, totalContainers := chunkProcessesAndContainers(procsByCtr, containers, maxBatchSize, maxBatchWeight)
	// fill in GroupSize for each CollectorProc and convert them to final messages
	// also count containers and processes
	messages := make([]model.MessageBody, 0, len(*collectorProcs))
	for idx := range *collectorProcs {
		m := &(*collectorProcs)[idx]
		m.GroupSize = int32(len(*collectorProcs))
		m.HostName = hostInfo.HostName
		m.NetworkId = networkID
		m.Info = hostInfo.SystemInfo
		m.GroupId = groupID
		m.ContainerHostType = hostInfo.ContainerHostType
		m.Hints = &model.CollectorProc_HintMask{HintMask: hints}

		messages = append(messages, m)
	}

	log.Tracef("Created %d process messages", len(messages))

	return messages, totalProcs, totalContainers
}

func chunkProcessesAndContainers(
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	maxChunkSize int,
	maxChunkWeight int,
) (*[]model.CollectorProc, int, int) {
	chunker := &util.ChunkAllocator[model.CollectorProc, *model.Process]{
		AppendToChunk: func(c *model.CollectorProc, ps []*model.Process) {
			c.Processes = append(c.Processes, ps...)
		},
	}

	totalProcs := len(procsByCtr[emptyCtrID])

	chunkProcessesBySizeAndWeight(procsByCtr[emptyCtrID], nil, maxChunkSize, maxChunkWeight, chunker)

	totalContainers := len(containers)
	for _, ctr := range containers {
		procs := procsByCtr[ctr.Id]
		totalProcs += len(procs)

		chunkProcessesBySizeAndWeight(procs, ctr, maxChunkSize, maxChunkWeight, chunker)
	}
	return chunker.GetChunks(), totalProcs, totalContainers
}

// fmtProcesses goes through each process, converts them to process object and group them by containers
// non-container processes would be in a single group with key as empty string ""
func fmtProcesses(
	scrubber *procutil.DataScrubber,
	disallowList []*regexp.Regexp,
	procs, lastProcs map[int32]*procutil.Process,
	ctrByProc map[int]string,
	syst2, syst1 cpu.TimesStat,
	lastRun time.Time,
	connRates ProcessConnRates,
) map[string][]*model.Process {
	procsByCtr := make(map[string][]*model.Process)

	for _, fp := range procs {
		if skipProcess(disallowList, fp, lastProcs) {
			continue
		}

		// Hide disallow-listed args if the Scrubber is enabled
		fp.Cmdline = scrubber.ScrubProcessCommand(fp)

		proc := &model.Process{
			Pid:                    fp.Pid,
			NsPid:                  fp.NsPid,
			Command:                formatCommand(fp),
			User:                   formatUser(fp),
			Memory:                 formatMemory(fp.Stats),
			Cpu:                    formatCPU(fp.Stats, lastProcs[fp.Pid].Stats, syst2, syst1),
			CreateTime:             fp.Stats.CreateTime,
			OpenFdCount:            fp.Stats.OpenFdCount,
			State:                  model.ProcessState(model.ProcessState_value[fp.Stats.Status]),
			IoStat:                 formatIO(fp.Stats, lastProcs[fp.Pid].Stats.IOStat, lastRun),
			VoluntaryCtxSwitches:   uint64(fp.Stats.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(fp.Stats.CtxSwitches.Involuntary),
			ContainerId:            ctrByProc[int(fp.Pid)],
		}

		if connRates != nil {
			proc.Networks = connRates[fp.Pid]
		}
		_, ok := procsByCtr[proc.ContainerId]
		if !ok {
			procsByCtr[proc.ContainerId] = make([]*model.Process, 0)
		}
		procsByCtr[proc.ContainerId] = append(procsByCtr[proc.ContainerId], proc)
	}

	scrubber.IncrementCacheAge()

	return procsByCtr
}

func formatCommand(fp *procutil.Process) *model.Command {
	return &model.Command{
		Args:   fp.Cmdline,
		Cwd:    fp.Cwd,
		Root:   "",    // TODO
		OnDisk: false, // TODO
		Ppid:   fp.Ppid,
		Exe:    fp.Exe,
	}
}

func formatIO(fp *procutil.Stats, lastIO *procutil.IOCountersStat, before time.Time) *model.IOStat {
	if fp.IORateStat != nil {
		return formatIORates(fp.IORateStat)
	}

	if fp.IOStat == nil { // This will be nil for Mac
		return &model.IOStat{}
	}

	diff := time.Now().Unix() - before.Unix()
	if before.IsZero() || diff <= 0 {
		return &model.IOStat{}
	}

	// Reading -1 as counter means the file could not be opened due to permissions.
	// In that case we set the rate as -1 to distinguish from a real 0 in rates.
	readRate := float32(-1)
	if fp.IOStat.ReadCount >= 0 {
		readRate = calculateRate(uint64(fp.IOStat.ReadCount), uint64(lastIO.ReadCount), before)
	}
	writeRate := float32(-1)
	if fp.IOStat.WriteCount >= 0 {
		writeRate = calculateRate(uint64(fp.IOStat.WriteCount), uint64(lastIO.WriteCount), before)
	}
	readBytesRate := float32(-1)
	if fp.IOStat.ReadBytes >= 0 {
		readBytesRate = calculateRate(uint64(fp.IOStat.ReadBytes), uint64(lastIO.ReadBytes), before)
	}
	writeBytesRate := float32(-1)
	if fp.IOStat.WriteBytes >= 0 {
		writeBytesRate = calculateRate(uint64(fp.IOStat.WriteBytes), uint64(lastIO.WriteBytes), before)
	}
	return &model.IOStat{
		ReadRate:       readRate,
		WriteRate:      writeRate,
		ReadBytesRate:  readBytesRate,
		WriteBytesRate: writeBytesRate,
	}
}

func formatIORates(ioRateStat *procutil.IOCountersRateStat) *model.IOStat {
	return &model.IOStat{
		ReadRate:       float32(ioRateStat.ReadRate),
		WriteRate:      float32(ioRateStat.WriteRate),
		ReadBytesRate:  float32(ioRateStat.ReadBytesRate),
		WriteBytesRate: float32(ioRateStat.WriteBytesRate),
	}
}

func formatMemory(fp *procutil.Stats) *model.MemoryStat {
	ms := &model.MemoryStat{
		Rss:  fp.MemInfo.RSS,
		Vms:  fp.MemInfo.VMS,
		Swap: fp.MemInfo.Swap,
	}

	if fp.MemInfoEx != nil {
		ms.Shared = fp.MemInfoEx.Shared
		ms.Text = fp.MemInfoEx.Text
		ms.Lib = fp.MemInfoEx.Lib
		ms.Data = fp.MemInfoEx.Data
		ms.Dirty = fp.MemInfoEx.Dirty
	}
	return ms
}

func formatNetworks(conns []*model.Connection, interval int) *model.ProcessNetworks {
	connRate := float32(len(conns)) / float32(interval)
	totalTraffic := uint64(0)
	for _, conn := range conns {
		totalTraffic += conn.LastBytesSent + conn.LastBytesReceived
	}
	bytesRate := float32(totalTraffic) / float32(interval)
	return &model.ProcessNetworks{ConnectionRate: connRate, BytesRate: bytesRate}
}

func formatCPU(statsNow, statsBefore *procutil.Stats, syst2, syst1 cpu.TimesStat) *model.CPUStat {
	if statsNow.CPUPercent != nil {
		return &model.CPUStat{
			LastCpu:   "cpu",
			TotalPct:  float32(statsNow.CPUPercent.UserPct + statsNow.CPUPercent.SystemPct),
			UserPct:   float32(statsNow.CPUPercent.UserPct),
			SystemPct: float32(statsNow.CPUPercent.SystemPct),
		}
	}
	return formatCPUTimes(statsNow, statsNow.CPUTime, statsBefore.CPUTime, syst2, syst1)
}

// skipProcess will skip a given process if it's disallow-listed or hasn't existed
// for multiple collections.
func skipProcess(
	disallowList []*regexp.Regexp,
	fp *procutil.Process,
	lastProcs map[int32]*procutil.Process,
) bool {
	cl := fp.Cmdline
	if len(cl) == 0 {
		cl = []string{fp.Exe}
		log.Debugf("Empty commandline for pid:%d using exe:[%s] to check if the process should be skipped", fp.Pid, cl)
	}
	if isDisallowListed(cl, disallowList) {
		return true
	}
	if _, ok := lastProcs[fp.Pid]; !ok {
		// Skipping any processes that didn't exist in the previous run.
		// This means short-lived processes (<2s) will never be captured.
		return true
	}
	return false
}

func (p *ProcessCheck) getRemoteSysProbeUtil() *net.RemoteSysProbeUtil {
	// if the Process module is disabled, we allow Probe to collect
	// fields that require elevated permission to collect with best effort
	if !p.SysprobeProcessModuleEnabled {
		return nil
	}

	pu, err := net.GetRemoteSystemProbeUtil()
	if err != nil {
		if p.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection in process check: %v (will only log every 10 minutes)", err)
		}
		return nil
	}
	return pu
}

// mergeProcWithSysprobeStats takes a process by PID map and fill the stats from system probe into the processes in the map
func mergeProcWithSysprobeStats(pids []int32, procs map[int32]*procutil.Process, pu net.SysProbeUtil) {
	pStats, err := pu.GetProcStats(pids)
	if err == nil {
		for pid, proc := range procs {
			if s, ok := pStats.StatsByPID[pid]; ok {
				proc.Stats.OpenFdCount = s.OpenFDCount
				proc.Stats.IOStat.ReadCount = s.ReadCount
				proc.Stats.IOStat.WriteCount = s.WriteCount
				proc.Stats.IOStat.ReadBytes = s.ReadBytes
				proc.Stats.IOStat.WriteBytes = s.WriteBytes
			}
		}
	} else {
		log.Debugf("cannot do GetProcStats from system-probe for process check: %s", err)
	}
}

func initScrubber(scrubber *procutil.DataScrubber) {
	// Enable/Disable the DataScrubber to obfuscate process args
	if ddconfig.Datadog.IsSet(configScrubArgs) {
		scrubber.Enabled = ddconfig.Datadog.GetBool(configScrubArgs)
	}

	if scrubber.Enabled { // Scrubber is enabled by default when it's created
		log.Debug("Starting process collection with Scrubber enabled")
	}

	// A custom word list to enhance the default one used by the DataScrubber
	if ddconfig.Datadog.IsSet(configCustomSensitiveWords) {
		words := ddconfig.Datadog.GetStringSlice(configCustomSensitiveWords)
		scrubber.AddCustomSensitiveWords(words)
		log.Debug("Adding custom sensitives words to Scrubber:", words)
	}

	// Strips all process arguments
	if ddconfig.Datadog.GetBool(configStripProcArgs) {
		log.Debug("Strip all process arguments enabled")
		scrubber.StripAllArguments = true
	}
}

func initDisallowList() []*regexp.Regexp {
	var disallowList []*regexp.Regexp
	// A list of regex patterns that will exclude a process if matched.
	if ddconfig.Datadog.IsSet(configDisallowList) {
		for _, b := range ddconfig.Datadog.GetStringSlice(configDisallowList) {
			r, err := regexp.Compile(b)
			if err != nil {
				log.Warnf("Ignoring invalid disallow list pattern: %s", b)
				continue
			}
			disallowList = append(disallowList, r)
		}
	}
	return disallowList
}

// isDisallowListed returns a boolean indicating if the given command is disallow-listed by our config.
func isDisallowListed(cmdline []string, disallowList []*regexp.Regexp) bool {
	cmd := strings.Join(cmdline, " ")
	for _, b := range disallowList {
		if b.MatchString(cmd) {
			return true
		}
	}
	return false
}
