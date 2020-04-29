package process

type Process struct {
	Pid  int32
	Ppid int32

	Name    string
	Cwd     string
	Exe     string
	Cmdline []string

	Username string // (Windows only)
	Uids     []int32
	Gids     []int32

	Stats *Stats
}

type Stats struct {
	// Status returns the process status.
	// Return value could be one of these.
	// R: Running S: Sleep T: Stop I: Idle
	// Z: Zombie W: Wait L: Lock
	// The character is same within all supported platforms.
	Status string

	CreateTime int64

	Nice        int32
	OpenFdCount int32
	NumThreads  int32

	CpuTime     *CPUTimesStat
	MemInfo     *MemoryInfoStat
	MemInfoEx   *MemoryInfoExStat
	IOStat      *IOCountersStat
	CtxSwitches *NumCtxSwitchesStat
}

type CPUTimesStat struct {
	CPU       string
	User      float64
	System    float64
	Idle      float64
	Nice      float64
	Iowait    float64
	Irq       float64
	Softirq   float64
	Steal     float64
	Guest     float64
	GuestNice float64
	Stolen    float64
	Timestamp int64
}

// Total returns the total number of seconds in a CPUTimesStat
func (c *CPUTimesStat) Total() float64 {
	total := c.User + c.System + c.Nice + c.Iowait + c.Irq + c.Softirq + c.Steal +
		c.Guest + c.GuestNice + c.Idle + c.Stolen
	return total
}

type MemoryInfoStat struct {
	RSS  uint64 // bytes
	VMS  uint64 // bytes
	Swap uint64 // bytes
}

type MemoryInfoExStat struct {
	RSS    uint64 // bytes
	VMS    uint64 // bytes
	Shared uint64 // bytes
	Text   uint64 // bytes
	Lib    uint64 // bytes
	Data   uint64 // bytes
	Dirty  uint64 // bytes
}

type IOCountersStat struct {
	ReadCount  uint64
	WriteCount uint64
	ReadBytes  uint64
	WriteBytes uint64
}

type NumCtxSwitchesStat struct {
	Voluntary   int64
	Involuntary int64
}
