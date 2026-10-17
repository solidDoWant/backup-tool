// Package retention decides which items in a time series to keep, under a policy made of cron schedules.
//
// Each rule's schedule splits time into periods: every scheduled time starts a period that lasts until the
// next scheduled time. A rule keeps the first (oldest) item in each of its N most recent periods, so
// "@weekly" (Sunday at midnight) keeps the Sunday item, or the item that stood in for it if Sunday's is
// missing. An item is kept if any rule keeps it.
//
// Periods are counted over the items themselves, not over the calendar: "keep 14 @daily" keeps an item from
// each of the 14 most recent days that have an item. If items stop being produced (e.g. backups start
// failing), nothing ages out until newer items replace it.
package retention

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/robfig/cron/v3"
)

// Unlimited keeps an item from every period of the rule's schedule.
const Unlimited = -1

// Rule keeps the first item in each of the Keep most recent periods of Schedule.
type Rule struct {
	// Schedule is a five-field cron expression (minute, hour, day of month, month, day of week) or a descriptor
	// (@yearly, @monthly, @weekly, @daily, @hourly). A field prefixed with "!" matches every value that the rest
	// of the field doesn't, e.g. "0 0 * * !sun" is every day but Sunday. Unlike cron, when both day of month and
	// day of week are restricted, a day must match both.
	Schedule string `yaml:"schedule" jsonschema:"required"`
	// Keep is the number of periods to keep an item from, or -1 for every period.
	Keep int `yaml:"keep" jsonschema:"required,minimum=-1"`
}

// Policy is a list of rules. An item is kept if any rule keeps it.
type Policy []Rule

// The bits of a cron.SpecSchedule field that are valid values, indexed by field position in a five-field
// expression. These match the bounds that the cron package uses.
var fieldValueMasks = [5]uint64{
	bitRange(0, 59), // Minute
	bitRange(0, 23), // Hour
	bitRange(1, 31), // Day of month
	bitRange(1, 12), // Month
	bitRange(0, 6),  // Day of week
}

// The cron package marks a day field that was given as "*" with this bit, and only requires both day fields
// to match when one of them has it.
const cronStarBit = 1 << 63

func bitRange(lowest, highest uint) uint64 {
	var bits uint64
	for i := lowest; i <= highest; i++ {
		bits |= 1 << i
	}
	return bits
}

// parseSchedule parses a rule's schedule, applying field negation and requiring both day fields to match.
func parseSchedule(schedule string, loc *time.Location) (*cron.SpecSchedule, error) {
	schedule = strings.TrimSpace(schedule)
	if strings.HasPrefix(schedule, "TZ=") || strings.HasPrefix(schedule, "CRON_TZ=") {
		return nil, trace.BadParameter("schedule %q sets a timezone; set the cleanup timezone instead", schedule)
	}

	fields := strings.Fields(schedule)
	var negated [len(fieldValueMasks)]bool
	if !strings.HasPrefix(schedule, "@") && len(fields) == len(fieldValueMasks) {
		for i, field := range fields {
			if trimmed, ok := strings.CutPrefix(field, "!"); ok {
				negated[i] = true
				fields[i] = trimmed
			}
		}
	}

	parsed, err := cron.ParseStandard(strings.Join(fields, " "))
	if err != nil {
		return nil, trace.Wrap(err, "invalid schedule %q", schedule)
	}

	spec, ok := parsed.(*cron.SpecSchedule)
	if !ok {
		// @every schedules repeat relative to when they're started, so they don't define fixed periods
		return nil, trace.BadParameter("schedule %q is not supported: only cron expressions and @yearly/@monthly/@weekly/@daily/@hourly can be used", schedule)
	}

	specFields := [len(fieldValueMasks)]*uint64{&spec.Minute, &spec.Hour, &spec.Dom, &spec.Month, &spec.Dow}
	for i, field := range specFields {
		if negated[i] {
			*field = ^*field & fieldValueMasks[i]
		}
	}

	spec.Dom |= cronStarBit
	spec.Location = loc

	return spec, nil
}

// Validate checks that the policy has at least one rule, and that every rule is valid. Requiring a rule means
// that applying a valid policy always keeps the newest item.
func (p Policy) Validate() error {
	if len(p) == 0 {
		return trace.BadParameter("at least one retention rule must be set, otherwise every item would be removed")
	}

	for i, rule := range p {
		if rule.Keep < 1 && rule.Keep != Unlimited {
			return trace.BadParameter("rule %d (%q): keep must be at least 1, or %d (unlimited), got %d", i, rule.Schedule, Unlimited, rule.Keep)
		}

		spec, err := parseSchedule(rule.Schedule, time.UTC)
		if err != nil {
			return trace.Wrap(err, "rule %d", i)
		}

		// Next gives up after five years
		if spec.Next(time.Now()).IsZero() {
			return trace.BadParameter("rule %d: schedule %q never matches", i, rule.Schedule)
		}
	}

	return nil
}

// Decision is the outcome of applying a policy to one item.
type Decision[T any] struct {
	Item T
	Time time.Time
	Keep bool
	// Reasons lists the schedules of the rules that keep the item. It is empty when the item is removed.
	Reasons []string
}

// Apply decides which items to keep. timeOf returns when an item was created, and schedules are evaluated
// in loc. Decisions are returned newest first. Items with identical times are ordered by their input order,
// so the result is deterministic for a given input.
func Apply[T any](p Policy, items []T, timeOf func(T) time.Time, loc *time.Location) ([]Decision[T], error) {
	decisions := make([]Decision[T], 0, len(items))
	for _, item := range items {
		decisions = append(decisions, Decision[T]{Item: item, Time: timeOf(item).In(loc)})
	}

	// Oldest first, so that the first item of each period is seen first
	slices.SortStableFunc(decisions, func(a, b Decision[T]) int {
		return cmp.Compare(a.Time.UnixNano(), b.Time.UnixNano())
	})

	for _, rule := range p {
		spec, err := parseSchedule(rule.Schedule, loc)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		// The index of the first item in each period. An item starts a new period if a scheduled time falls
		// after the previous item and at or before it.
		var periodStarts []int
		for i := range decisions {
			if i == 0 {
				periodStarts = append(periodStarts, i)
				continue
			}

			next := spec.Next(decisions[i-1].Time)
			if !next.IsZero() && !next.After(decisions[i].Time) {
				periodStarts = append(periodStarts, i)
			}
		}

		// Keep the first item of the most recent periods
		kept := 0
		for _, i := range slices.Backward(periodStarts) {
			if rule.Keep != Unlimited && kept == rule.Keep {
				break
			}

			decisions[i].Keep = true
			decisions[i].Reasons = append(decisions[i].Reasons, rule.Schedule)
			kept++
		}
	}

	slices.Reverse(decisions)
	return decisions, nil
}
