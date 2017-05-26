package system

import (
	"runtime"
	"testing"

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

func ioSampler() (map[string]disk.IOCountersStat, error) {
	idx := sampleIdx
	sampleIdx = sampleIdx + 1
	sampleIdx = sampleIdx % len(ioSamples)
	return ioSamples[idx], nil
}

func TestIOCheckLinux(t *testing.T) {
	ioCounters = ioSampler
	ioCheck := new(IOCheck)
	ioCheck.Configure(nil, nil)

	mock := new(MockSender)
	ioCheck.sender = mock

	expectedCalls := 4
	switch os := runtime.GOOS; os {
	case "windows":
		mock.On("Gauge", "system.io.r_s", 443071.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_s", 10412454.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.rkb_s", float64(849293*SectorSize), "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.wkb_s", float64(1406995*SectorSize), "", []string{"device:sda"}).Return().Times(1)
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Gauge", "system.io.r_s", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_s", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.rkb_s", 60.5, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.wkb_s", 37.5, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.avg_rq_sz", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.await", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.r_await", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_await", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.rrqm_s", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.wrqm_s", 0.0, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.avg_q_sz", 0.028, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.util", 2.8, "", []string{"device:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.svctm", 0.0, "", []string{"device:sda"}).Return().Times(1)
		expectedCalls += 9
	}

	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", expectedCalls)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
