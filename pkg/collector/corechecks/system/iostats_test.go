package system

import (
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/shirou/gopsutil/disk"
)

var (
	ioSamples = []map[string]disk.IOCountersStat{
		map[string]disk.IOCountersStat{
			"sda": disk.IOCountersStat{
				ReadCount:        443071,
				MergedReadCount:  104744,
				WriteCount:       10412454,
				MergedWriteCount: 310860,
				ReadBytes:        849293 * SectorSize,
				WriteBytes:       1406995 * SectorSize,
				ReadTime:         19699308,
				WriteTime:        418600,
				IopsInProgress:   0,
				IoTime:           343324,
				WeightedIO:       727464,
				Name:             "sda",
				SerialNumber:     "123456789WD",
			},
		}, map[string]disk.IOCountersStat{
			"sda": disk.IOCountersStat{
				ReadCount:        443071,
				MergedReadCount:  104744,
				WriteCount:       10412454,
				MergedWriteCount: 310860,
				ReadBytes:        849414 * SectorSize,
				WriteBytes:       1407070 * SectorSize,
				ReadTime:         19700964,
				WriteTime:        418628,
				IopsInProgress:   0,
				IoTime:           343352,
				WeightedIO:       727492,
				Name:             "sda",
				SerialNumber:     "123456789WD",
			},
		},
	}
)

var sampleIdx = 0

func ioSampler(names ...string) (map[string]disk.IOCountersStat, error) {
	idx := sampleIdx
	sampleIdx++
	sampleIdx = sampleIdx % len(ioSamples)
	return ioSamples[idx], nil
}

func TestIOCheck(t *testing.T) {
	ioCounters = ioSampler
	ioCheck := new(IOCheck)
	ioCheck.Configure(nil, nil)

	mock := new(MockSender)
	ioCheck.sender = mock

	expectedRates := 2
	expectedGauges := 0

	switch os := runtime.GOOS; os {
	case "windows":
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:sda"}).Return().Times(1)
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.rrqm_s", 104744.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.wrqm_s", 310860.0, "", []string{"device:sda"}).Return().Times(1)
		expectedRates += 2
	}
	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)

	// sleep for a second, for delta
	time.Sleep(time.Second)

	switch os := runtime.GOOS; os {
	case "windows":
		mock.On("Gauge", "system.io.r_s", 443071.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_s", 10412454.0, "", []string{"device:sda"}).Return().Times(1)
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.rrqm_s", 104744.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.wrqm_s", 310860.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.rkb_s", 60.5, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.wkb_s", 37.5, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.avg_rq_sz", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.await", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.r_await", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_await", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.avg_q_sz", 0.028, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.util", 2.8, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.svctm", 0.0, "", []string{"device:sda"}).Return().Times(1)
		expectedRates += 4
		expectedGauges += 9
	}

	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 2)
}

func TestIOCheckBlacklist(t *testing.T) {
	ioCounters = ioSampler
	ioCheck := new(IOCheck)
	ioCheck.Configure(nil, nil)

	mock := new(MockSender)
	ioCheck.sender = mock

	//set blacklist
	bl, err := regexp.Compile("sd.*")
	if err != nil {
		t.FailNow()
	}
	ioCheck.blacklist = bl

	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 0)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
