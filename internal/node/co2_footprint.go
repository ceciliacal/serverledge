package node

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CarbonFootprint stores the current carbon intensity for the local node.
// Intensity is expressed as grams of CO2 equivalent per kWh.
type CarbonFootprint struct {
	mu  sync.RWMutex
	t   time.Time
	val float64
	idx int
}

func (c *CarbonFootprint) Intensity() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.val
}

func (c *CarbonFootprint) Set(t time.Time, intensity float64, nextIndex int) {
	c.mu.Lock()
	c.t = t
	c.val = intensity
	c.idx = nextIndex
	c.mu.Unlock()
}

func (c *CarbonFootprint) Snapshot() (time.Time, float64, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.t, c.val, c.idx
}

type CO2Sample struct {
	Time      time.Time
	Intensity float64
}

type CO2Trace struct {
	samples []CO2Sample
	next    int
}

func LoadCO2TraceFromCSV(csvPath, timeCol, intensityCol string, loc *time.Location) (*CO2Trace, error) {
	samples, err := loadCO2Samples(csvPath, timeCol, intensityCol, loc)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, errors.New("CO2: CSV has no data rows")
	}
	return &CO2Trace{samples: samples}, nil
}

func (t *CO2Trace) Samples() []CO2Sample {
	out := make([]CO2Sample, len(t.samples))
	copy(out, t.samples)
	return out
}

// ApplyNext advances one trace row into the supplied footprint. It returns
// false once the trace is exhausted; the last applied value remains current.
func (t *CO2Trace) ApplyNext(c *CarbonFootprint) bool {
	if t.next >= len(t.samples) {
		return false
	}
	s := t.samples[t.next]
	t.next++
	c.Set(s.Time, s.Intensity, t.next)
	return true
}

// StartCO2FromCSV loads a trace, applies the first row immediately, then
// advances one row on each period. When the trace is exhausted the updater
// stops and leaves the last intensity in place.
func StartCO2FromCSV(parent context.Context, csvPath, timeCol, intensityCol string, period time.Duration, loc *time.Location) (func(), error) {
	if period <= 0 {
		return nil, fmt.Errorf("CO2: update period must be positive")
	}
	trace, err := LoadCO2TraceFromCSV(csvPath, timeCol, intensityCol, loc)
	if err != nil {
		return nil, err
	}
	trace.ApplyNext(&LocalResources.Co2Footprint)

	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	var closeDone sync.Once

	go func() {
		defer closeDone.Do(func() { close(done) })
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !trace.ApplyNext(&LocalResources.Co2Footprint) {
					return
				}
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}, nil
}

func loadCO2Samples(csvPath, timeCol, intensityCol string, loc *time.Location) ([]CO2Sample, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("CO2: open CSV: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CO2: read header: %w", err)
	}
	ti := findCSVColumn(header, timeCol)
	ii := findCSVColumn(header, intensityCol)
	if ti < 0 || ii < 0 {
		return nil, fmt.Errorf("CO2: columns not found: time=%q intensity=%q", timeCol, intensityCol)
	}

	var out []CO2Sample
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CO2: read row: %w", err)
		}
		if ti >= len(rec) || ii >= len(rec) {
			return nil, fmt.Errorf("CO2: incomplete row")
		}

		tm, err := parseCO2Time(strings.TrimSpace(rec[ti]), loc)
		if err != nil {
			return nil, fmt.Errorf("CO2: parse time %q: %w", strings.TrimSpace(rec[ti]), err)
		}
		intensity, err := strconv.ParseFloat(strings.TrimSpace(rec[ii]), 64)
		if err != nil {
			return nil, fmt.Errorf("CO2: parse intensity %q: %w", strings.TrimSpace(rec[ii]), err)
		}
		out = append(out, CO2Sample{Time: tm, Intensity: intensity})
	}
	return out, nil
}

func findCSVColumn(header []string, want string) int {
	want = strings.ToLower(strings.TrimSpace(want))
	for i, h := range header {
		if strings.ToLower(strings.TrimSpace(h)) == want {
			return i
		}
	}
	return -1
}

func parseCO2Time(s string, loc *time.Location) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	var first error
	for _, layout := range layouts {
		if loc != nil && layout != time.RFC3339 {
			t, err := time.ParseInLocation(layout, s, loc)
			if err == nil {
				return t, nil
			}
			if first == nil {
				first = err
			}
			continue
		}
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		if first == nil {
			first = err
		}
	}
	return time.Time{}, first
}
