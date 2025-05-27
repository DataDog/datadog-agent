package debugging

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type RollingLog struct {
	mu      sync.Mutex
	logList []string
	size    uint64
	maxSize uint64
}

func (a *RollingLog) removeUntilFits(size uint64) error {
	if a.maxSize-a.size >= size {
		return nil
	}

	if size > a.maxSize {
		return fmt.Errorf("size exceeds max size: %d", a.maxSize)
	}

	for {
		if len(a.logList) == 0 {
			return nil
		}

		a.size -= uint64(len(a.logList[0]))
		a.logList = a.logList[1:]
		if a.maxSize-a.size >= size {
			return nil
		}
	}
}

func NewRollingLog(maxSize uint64) *RollingLog {
	return &RollingLog{
		logList: make([]string, 0),
		maxSize: maxSize,
		size:    0,
	}
}

func (a *RollingLog) Add(str string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	str = "[" + time.Now().Format("2006-01-02 15:04:05.0000000") + "] " + str
	sz := uint64(len(str))
	err := a.removeUntilFits(sz)
	if err != nil {
		fmt.Println("Error: ", err)
	}
	a.size += sz
	a.logList = append(a.logList, str)
}

func (a *RollingLog) Get() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.logList, "\n")
}

func (a *RollingLog) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.size = 0
	a.logList = make([]string, 0)
}
