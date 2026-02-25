package interp

import (
	"math"
	"testing"
)

func TestPingStatsLoss(t *testing.T) {
	tests := []struct {
		name     string
		sent     int
		received int
		want     float64
	}{
		{"no loss", 4, 4, 0},
		{"25% loss", 4, 3, 25},
		{"100% loss", 4, 0, 100},
		{"50% loss", 10, 5, 50},
		{"zero sent", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &pingStats{sent: tt.sent, received: tt.received}
			if got := s.loss(); got != tt.want {
				t.Errorf("loss() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPingStatsMinMaxAvgMdev(t *testing.T) {
	s := &pingStats{
		rtts: []float64{10.0, 20.0, 30.0, 40.0},
	}

	if got := s.min(); got != 10.0 {
		t.Errorf("min() = %v, want 10.0", got)
	}
	if got := s.max(); got != 40.0 {
		t.Errorf("max() = %v, want 40.0", got)
	}
	if got := s.avg(); got != 25.0 {
		t.Errorf("avg() = %v, want 25.0", got)
	}
	// mdev = mean(|10-25|, |20-25|, |30-25|, |40-25|) = mean(15, 5, 5, 15) = 10
	if got := s.mdev(); math.Abs(got-10.0) > 0.001 {
		t.Errorf("mdev() = %v, want 10.0", got)
	}
}

func TestPingStatsSingleValue(t *testing.T) {
	s := &pingStats{
		rtts: []float64{42.5},
	}

	if got := s.min(); got != 42.5 {
		t.Errorf("min() = %v, want 42.5", got)
	}
	if got := s.max(); got != 42.5 {
		t.Errorf("max() = %v, want 42.5", got)
	}
	if got := s.avg(); got != 42.5 {
		t.Errorf("avg() = %v, want 42.5", got)
	}
	if got := s.mdev(); got != 0.0 {
		t.Errorf("mdev() = %v, want 0.0", got)
	}
}

func TestPingStatsEmpty(t *testing.T) {
	s := &pingStats{}

	if got := s.avg(); got != 0 {
		t.Errorf("avg() = %v, want 0", got)
	}
	if got := s.mdev(); got != 0 {
		t.Errorf("mdev() = %v, want 0", got)
	}
	if got := s.loss(); got != 0 {
		t.Errorf("loss() = %v, want 0", got)
	}
}
