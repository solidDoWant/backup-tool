package disasterrecovery

import (
	"fmt"
	"strings"
	"time"
)

type DREvent struct {
	Name      string // Human-readable, DNS-safe name to identify the DR event. Must be unique for a given timestamp (within 1s).
	StartTime time.Time
	EndTime   time.Time
}

func NewDREventNow(name string) *DREvent {
	return &DREvent{
		Name:      name,
		StartTime: time.Now(),
	}
}

func (b *DREvent) GetFullName() string {
	// Example: "mybackup-2006-01-02T15.04.05Z"
	return fmt.Sprintf("%s-%s", b.Name, strings.ReplaceAll(b.StartTime.UTC().Format(time.RFC3339), ":", "."))
}

func (b *DREvent) Stop() {
	b.EndTime = time.Now()
}

func (b *DREvent) HasCompleted() bool {
	return !b.EndTime.IsZero()
}

func (b *DREvent) CalculateRuntime() time.Duration {
	lastRunningTime := b.EndTime
	if !b.HasCompleted() {
		lastRunningTime = time.Now()
	}

	return lastRunningTime.Sub(b.StartTime)
}
