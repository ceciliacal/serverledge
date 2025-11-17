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

// ---- Data that lives on the node ----

type CarbonFootprint struct {
	mu  sync.RWMutex
	t   time.Time
	val float64
	idx int // next row index to consume
}

func (c *CarbonFootprint) Intensity() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.val
}

func (c *CarbonFootprint) set(t time.Time, v float64, nextIdx int) {
	c.mu.Lock()
	c.t = t
	c.val = v
	c.idx = nextIdx
	c.mu.Unlock()
}

// ---- CSV-based updater ----

type co2Row struct {
	t   time.Time
	val float64
}

// StartCO2FromCSV loads a time series and periodically updates LocalResources.Co2Footprint.
// It sets the first value immediately (no initial zeros). It returns a stop() you should call on shutdown.
func StartCO2FromCSV(ctx context.Context, csvPath, timeCol, intensityCol string, period time.Duration, loc *time.Location) (stop func(), err error) {
	rows, err := loadRows(csvPath, timeCol, intensityCol, loc)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("CO2: CSV has no data rows")
	}

	// Write first value immediately
	LocalResources.Co2Footprint.set(rows[0].t, rows[0].val, 1)

	// Ticker loop
	ticker := time.NewTicker(period)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				LocalResources.Co2Footprint.mu.Lock()
				i := LocalResources.Co2Footprint.idx
				if i >= len(rows) {
					// stop when CSV is exhausted
					LocalResources.Co2Footprint.mu.Unlock()
					close(done)
					return
				}
				r := rows[i]
				LocalResources.Co2Footprint.t = r.t
				LocalResources.Co2Footprint.val = r.val
				LocalResources.Co2Footprint.idx = i + 1
				LocalResources.Co2Footprint.mu.Unlock()
			}
		}
	}()

	stop = func() {
		// cancel via context; wait for goroutine to exit
		if cancel := ctx.Value(co2CancelKey{}); cancel != nil {
			if f, ok := cancel.(context.CancelFunc); ok {
				f()
			}
		}
		<-done
	}
	return stop, nil
}

type co2CancelKey struct{}

func loadRows(csvPath, timeCol, intensityCol string, loc *time.Location) ([]co2Row, error) {
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
	ti := findIdx(header, timeCol)
	ii := findIdx(header, intensityCol)
	if ti < 0 || ii < 0 {
		return nil, fmt.Errorf("CO2: columns not found: time=%q intensity=%q", timeCol, intensityCol)
	}

	var out []co2Row
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CO2: read row: %w", err)
		}
		ts := strings.TrimSpace(rec[ti])
		is := strings.TrimSpace(rec[ii])

		tm, err := parseTime(ts, loc)
		if err != nil {
			return nil, fmt.Errorf("CO2: parse time %q: %w", ts, err)
		}
		val, err := strconv.ParseFloat(is, 64)
		if err != nil {
			return nil, fmt.Errorf("CO2: parse intensity %q: %w", is, err)
		}
		out = append(out, co2Row{t: tm, val: val})
	}
	return out, nil
}

func findIdx(header []string, want string) int {
	want = strings.ToLower(strings.TrimSpace(want))
	for i, h := range header {
		if strings.ToLower(strings.TrimSpace(h)) == want {
			return i
		}
	}
	return -1
}

func parseTime(s string, loc *time.Location) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	var first error
	for _, l := range layouts {
		if loc != nil && (l == "2006-01-02 15:04:05" || l == "2006-01-02 15:04" || l == "2006-01-02") {
			if t, err := time.ParseInLocation(l, s, loc); err == nil {
				return t, nil
			} else if first == nil {
				first = err
			}
			continue
		}
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		} else if first == nil {
			first = err
		}
	}
	return time.Time{}, first
}

func (c *CarbonFootprint) Snapshot() (t time.Time, intensity float64, idx int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.t, c.val, c.idx
}
