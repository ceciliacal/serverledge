package node

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CarbonFootprint struct {
	mu           sync.RWMutex
	co2Datetime  time.Time
	co2Intensity float64
	count        int // row index we've consumed
}

// Snapshot returns a copy of current state.
func (c *CarbonFootprint) Snapshot() (t time.Time, intensity float64, count int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.co2Datetime, c.co2Intensity, c.count
}

type CO2Poller struct {
	rows      []co2Row
	cf        *CarbonFootprint
	cancel    context.CancelFunc
	runningMu sync.Mutex
}

type co2Row struct {
	t         time.Time
	intensity float64
}

func (p *CO2Poller) Start(csvPath, timeCol, intensityCol string, period time.Duration, loc *time.Location) error {
	p.runningMu.Lock()
	defer p.runningMu.Unlock()
	if p.cancel != nil {
		return errors.New("poller already running")
	}

	rows, err := loadRows(csvPath, timeCol, intensityCol, loc)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("no data rows in CSV")
	}
	p.rows = rows
	if p.cf == nil {
		p.cf = &CarbonFootprint{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	ticker := time.NewTicker(period)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.step()
			}
		}
	}()

	p.step()
	return nil
}

func (p *CO2Poller) step() {
	p.cf.mu.Lock()
	defer p.cf.mu.Unlock()

	if p.cf.count >= len(p.rows) {
		// stop automatically when CSV is exhausted
		if p.cancel != nil {
			p.cancel()
			p.cancel = nil
		}
		return
	}
	r := p.rows[p.cf.count]
	p.cf.co2Datetime = r.t
	p.cf.co2Intensity = r.intensity
	p.cf.count++
}

func (p *CO2Poller) Stop() {
	p.runningMu.Lock()
	defer p.runningMu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

func (p *CO2Poller) CarbonFootprint() *CarbonFootprint {
	return p.cf
}

func loadRows(csvPath, timeCol, intensityCol string, loc *time.Location) ([]co2Row, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	ti := findIdx(header, timeCol)
	ii := findIdx(header, intensityCol)
	if ti < 0 || ii < 0 {
		return nil, errors.New("timestamp or intensity column not found")
	}

	var out []co2Row
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		ts := strings.TrimSpace(rec[ti])
		ints := strings.TrimSpace(rec[ii])

		tm, err := parseTime(ts, loc)
		if err != nil {
			return nil, err
		}
		val, err := strconv.ParseFloat(ints, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, co2Row{t: tm, intensity: val})
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

// adjust/extend layouts to match your CSVs (FR_2023_hourly.csv, PL_2023_hourly.csv)
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
