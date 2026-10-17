package retention

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyValidate(t *testing.T) {
	tests := []struct {
		desc    string
		policy  Policy
		wantErr bool
	}{
		{
			desc:    "no rules",
			wantErr: true,
		},
		{
			desc:   "single rule",
			policy: Policy{{Schedule: "@daily", Keep: 7}},
		},
		{
			desc:   "unlimited",
			policy: Policy{{Schedule: "@yearly", Keep: Unlimited}},
		},
		{
			desc: "cron expressions",
			policy: Policy{
				{Schedule: "0 0 * * *", Keep: 14},
				{Schedule: "0 0 * * sun", Keep: 4},
				{Schedule: "0 0 1 */3 *", Keep: 4},
				{Schedule: "30 2 1-7 * mon", Keep: 2},
			},
		},
		{
			desc:   "negated fields",
			policy: Policy{{Schedule: "0 0 * * !0", Keep: 6}, {Schedule: "0 0 1 !jan,jul *", Keep: 10}},
		},
		{
			desc:    "zero keep",
			policy:  Policy{{Schedule: "@daily", Keep: 0}},
			wantErr: true,
		},
		{
			desc:    "below unlimited",
			policy:  Policy{{Schedule: "@daily", Keep: -2}},
			wantErr: true,
		},
		{
			desc:    "invalid expression",
			policy:  Policy{{Schedule: "0 0 * *", Keep: 1}},
			wantErr: true,
		},
		{
			desc:    "invalid negated field",
			policy:  Policy{{Schedule: "0 0 * * !", Keep: 1}},
			wantErr: true,
		},
		{
			desc:    "every schedules are relative",
			policy:  Policy{{Schedule: "@every 24h", Keep: 1}},
			wantErr: true,
		},
		{
			desc:    "per-schedule timezone",
			policy:  Policy{{Schedule: "CRON_TZ=America/Chicago 0 0 * * *", Keep: 1}},
			wantErr: true,
		},
		{
			desc:    "never matches",
			policy:  Policy{{Schedule: "0 0 31 feb *", Keep: 1}},
			wantErr: true,
		},
		{
			desc:    "negating everything never matches",
			policy:  Policy{{Schedule: "0 0 * * !*", Keep: 1}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseSchedule(t *testing.T) {
	// 2026-01-04 is a Sunday
	sunday := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		desc     string
		schedule string
		from     time.Time
		wantNext []time.Time
	}{
		{
			desc:     "descriptor",
			schedule: "@weekly",
			from:     sunday,
			wantNext: []time.Time{sunday.AddDate(0, 0, 7), sunday.AddDate(0, 0, 14)},
		},
		{
			desc:     "negated day of week",
			schedule: "0 0 * * !sun",
			from:     sunday.AddDate(0, 0, -1),
			wantNext: []time.Time{sunday.AddDate(0, 0, 1), sunday.AddDate(0, 0, 2)},
		},
		{
			desc:     "negated list of months",
			schedule: "0 0 1 !feb-nov *",
			from:     time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC),
			wantNext: []time.Time{
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			desc:     "negated hour",
			schedule: "0 !1-23 * * *",
			from:     sunday,
			wantNext: []time.Time{sunday.AddDate(0, 0, 1), sunday.AddDate(0, 0, 2)},
		},
		{
			// Cron would match the 1st of the month or any Sunday. 2026-02-01 and 2026-03-01 are Sundays.
			desc:     "both day fields must match",
			schedule: "0 0 1 * sun",
			from:     sunday,
			wantNext: []time.Time{time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			desc:     "first of the month unless it's a Sunday", // Skips February and March
			schedule: "0 0 1 * !sun",
			from:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			wantNext: []time.Time{time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			spec, err := parseSchedule(tt.schedule, time.UTC)
			require.NoError(t, err)

			next := tt.from
			for _, want := range tt.wantNext {
				next = spec.Next(next)
				assert.Equal(t, want, next)
			}
		})
	}
}

func date(t *testing.T, value string) time.Time {
	parsed, err := time.Parse(time.DateOnly, value)
	require.NoError(t, err)
	return parsed
}

// nightly returns one item per day from start to end inclusive, taken shortly after midnight UTC.
func nightly(t *testing.T, start, end string) []time.Time {
	var items []time.Time
	for day := date(t, start); !day.After(date(t, end)); day = day.AddDate(0, 0, 1) {
		items = append(items, day.Add(5*time.Minute))
	}
	return items
}

func identity(t time.Time) time.Time { return t }

func keptDates(decisions []Decision[time.Time]) []string {
	var kept []string
	for _, decision := range decisions {
		if decision.Keep {
			kept = append(kept, decision.Time.Format(time.DateOnly))
		}
	}
	return kept
}

func TestApply(t *testing.T) {
	tests := []struct {
		desc     string
		policy   Policy
		items    []time.Time
		loc      *time.Location
		wantKept []string
	}{
		{
			desc:   "no items",
			policy: Policy{{Schedule: "@daily", Keep: 7}},
		},
		{
			desc:     "fewer periods than the rule allows",
			policy:   Policy{{Schedule: "@daily", Keep: 30}},
			items:    nightly(t, "2026-01-01", "2026-01-03"),
			wantKept: []string{"2026-01-03", "2026-01-02", "2026-01-01"},
		},
		{
			desc:   "keeps the first item in each period",
			policy: Policy{{Schedule: "@daily", Keep: 2}},
			items: []time.Time{
				date(t, "2026-01-01").Add(1 * time.Hour),
				date(t, "2026-01-02").Add(2 * time.Hour),
				date(t, "2026-01-02").Add(1 * time.Hour),
				date(t, "2026-01-03").Add(1 * time.Hour),
			},
			wantKept: []string{"2026-01-03", "2026-01-02"},
		},
		{
			// 2026-01-04, 2026-01-11, 2026-01-18 are Sundays
			desc:     "weekly keeps the Sunday item",
			policy:   Policy{{Schedule: "@weekly", Keep: 3}},
			items:    nightly(t, "2026-01-01", "2026-01-21"),
			wantKept: []string{"2026-01-18", "2026-01-11", "2026-01-04"},
		},
		{
			desc:     "a missing item is stood in for by the next one",
			policy:   Policy{{Schedule: "@weekly", Keep: 2}},
			items:    append(nightly(t, "2026-01-05", "2026-01-10"), nightly(t, "2026-01-12", "2026-01-14")...),
			wantKept: []string{"2026-01-12", "2026-01-05"},
		},
		{
			desc:     "monthly keeps the first item of each month",
			policy:   Policy{{Schedule: "@monthly", Keep: 3}},
			items:    nightly(t, "2025-11-15", "2026-02-10"),
			wantKept: []string{"2026-02-01", "2026-01-01", "2025-12-01"},
		},
		{
			desc:     "unlimited yearly",
			policy:   Policy{{Schedule: "@yearly", Keep: Unlimited}},
			items:    nightly(t, "2022-06-01", "2026-03-01"),
			wantKept: []string{"2026-01-01", "2025-01-01", "2024-01-01", "2023-01-01", "2022-06-01"},
		},
		{
			desc:     "every item",
			policy:   Policy{{Schedule: "* * * * *", Keep: 3}},
			items:    nightly(t, "2026-01-01", "2026-01-10"),
			wantKept: []string{"2026-01-10", "2026-01-09", "2026-01-08"},
		},
		{
			// 2026-01-04 and 2026-01-11 are Sundays. Sunday items fall in the period started by Saturday's
			// scheduled time, so they are never the first item of a period.
			desc:     "negated field",
			policy:   Policy{{Schedule: "0 0 * * !sun", Keep: Unlimited}},
			items:    nightly(t, "2026-01-03", "2026-01-12"),
			wantKept: []string{"2026-01-12", "2026-01-10", "2026-01-09", "2026-01-08", "2026-01-07", "2026-01-06", "2026-01-05", "2026-01-03"},
		},
		{
			desc:   "periods count periods with items, not calendar periods",
			policy: Policy{{Schedule: "@daily", Keep: 3}},
			items: []time.Time{
				date(t, "2026-01-01"),
				date(t, "2026-01-02"),
				// Gap: no backups for weeks
				date(t, "2026-03-01"),
			},
			wantKept: []string{"2026-03-01", "2026-01-02", "2026-01-01"},
		},
		{
			// 23:30 UTC on Jan 1 is Jan 2 in UTC+1, so it starts Jan 2's period there and the noon item doesn't
			desc:   "schedules are evaluated in the given location",
			policy: Policy{{Schedule: "@daily", Keep: Unlimited}},
			items: []time.Time{
				date(t, "2026-01-01").Add(23*time.Hour + 30*time.Minute),
				date(t, "2026-01-02").Add(12 * time.Hour),
			},
			loc:      time.FixedZone("UTC+1", 60*60),
			wantKept: []string{"2026-01-02"},
		},
		{
			// The policy from the feature request: two weeks of nightlies, a Sunday for every week in a
			// month, one for every month in a year, and one for every year. 2026-10-17 is a Saturday.
			desc: "grandfather-father-son",
			policy: Policy{
				{Schedule: "@daily", Keep: 14},
				{Schedule: "@weekly", Keep: 4},
				{Schedule: "@monthly", Keep: 12},
				{Schedule: "@yearly", Keep: Unlimited},
			},
			items: nightly(t, "2024-06-01", "2026-10-17"),
			wantKept: []string{
				// Daily (10-11 and 10-04 are also weekly)
				"2026-10-17", "2026-10-16", "2026-10-15", "2026-10-14", "2026-10-13", "2026-10-12", "2026-10-11",
				"2026-10-10", "2026-10-09", "2026-10-08", "2026-10-07", "2026-10-06", "2026-10-05", "2026-10-04",
				// Weekly
				"2026-09-27", "2026-09-20",
				// Monthly (2026-01-01 is also yearly)
				"2026-10-01", "2026-09-01", "2026-08-01", "2026-07-01", "2026-06-01", "2026-05-01", "2026-04-01",
				"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01",
				// Yearly. Backups started mid-2024, so its first one stands in for the year.
				"2025-01-01", "2024-06-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			loc := tt.loc
			if loc == nil {
				loc = time.UTC
			}

			require.NoError(t, tt.policy.Validate())
			decisions, err := Apply(tt.policy, tt.items, identity, loc)
			require.NoError(t, err)
			require.Len(t, decisions, len(tt.items))
			assert.ElementsMatch(t, tt.wantKept, keptDates(decisions))

			for i, decision := range decisions {
				if i > 0 {
					assert.False(t, decision.Time.After(decisions[i-1].Time), "decisions should be sorted newest first")
				}

				if decision.Keep {
					assert.NotEmpty(t, decision.Reasons)
				} else {
					assert.Empty(t, decision.Reasons)
				}
			}
		})
	}
}

func TestApplyReasons(t *testing.T) {
	items := nightly(t, "2025-12-31", "2026-01-02")
	decisions, err := Apply(Policy{{Schedule: "@daily", Keep: 2}, {Schedule: "@monthly", Keep: 2}, {Schedule: "@yearly", Keep: 2}}, items, identity, time.UTC)
	require.NoError(t, err)

	reasons := make(map[string][]string)
	for _, decision := range decisions {
		reasons[decision.Time.Format(time.DateOnly)] = decision.Reasons
	}

	assert.Equal(t, map[string][]string{
		"2026-01-02": {"@daily"},
		"2026-01-01": {"@daily", "@monthly", "@yearly"},
		"2025-12-31": {"@monthly", "@yearly"},
	}, reasons)
}

func TestApplyInvalidSchedule(t *testing.T) {
	_, err := Apply(Policy{{Schedule: "not a schedule", Keep: 1}}, nightly(t, "2026-01-01", "2026-01-02"), identity, time.UTC)
	assert.Error(t, err)
}

func TestApplyIsStableForEqualTimes(t *testing.T) {
	type item struct {
		name string
		at   time.Time
	}

	at := date(t, "2026-01-01")
	items := []item{{"a", at}, {"b", at}, {"c", at}}
	decisions, err := Apply(Policy{{Schedule: "@daily", Keep: 1}}, items, func(i item) time.Time { return i.at }, time.UTC)
	require.NoError(t, err)

	require.Len(t, decisions, 3)
	kept := make(map[string]bool)
	for _, decision := range decisions {
		kept[decision.Item.name] = decision.Keep
	}
	// The first item of the period, in input order, is kept
	assert.Equal(t, map[string]bool{"a": true, "b": false, "c": false}, kept)
}
