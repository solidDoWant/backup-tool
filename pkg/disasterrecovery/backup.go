package disasterrecovery

import (
	"fmt"
	"strings"
	"time"
)

type Backup struct {
	Name      string // Human-readable, DNS-safe name to identify the backup. Must be unique for a given timestamp (within 1s).
	StartTime time.Time
	EndTime   time.Time
}

func NewBackupNow(name string) *Backup {
	return &Backup{
		Name:      name,
		StartTime: time.Now(),
	}
}

func (b *Backup) GetFullName() string {
	// Example: "mybackup-2006-01-02T15.04.05Z"
	return fmt.Sprintf("%s-%s", b.Name, strings.ReplaceAll(b.StartTime.UTC().Format(time.RFC3339), ":", "."))
}

func (b *Backup) Stop() {
	b.EndTime = time.Now()
}

func (b *Backup) HasCompleted() bool {
	return !b.EndTime.IsZero()
}

func (b *Backup) CalculateRuntime() time.Duration {
	lastRunningTime := b.EndTime
	if !b.HasCompleted() {
		lastRunningTime = time.Now()
	}

	return lastRunningTime.Sub(b.StartTime)
}
