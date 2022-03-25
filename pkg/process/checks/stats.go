package checks

import "time"

type Stats struct {
	CheckName         string
	CheckConfigSource string
	Runtime           time.Duration
	IsRealtime        bool
}

func NewStats(c Check) *Stats {
	return &Stats{
		CheckName:  c.Name(),
		IsRealtime: false,
	}
}

func NewStatsRealtime(c CheckWithRealTime) *Stats {
	return &Stats{
		CheckName:  c.RealTimeName(),
		IsRealtime: true,
	}
}
