package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{value: "5min", want: 5 * time.Minute},
		{value: "30s", want: 30 * time.Second},
		{value: "1h", want: time.Hour},
		{value: "2d", want: 48 * time.Hour},
		{value: "500ms", want: 500 * time.Millisecond},
		{value: "100us", want: 100 * time.Microsecond},
		{value: "300", want: 300 * time.Second},  // bare integer defaults to seconds
		{value: " 5min ", want: 5 * time.Minute}, // surrounding whitespace tolerated
		{value: "0", want: 0},
		{value: "", wantErr: true},
		{value: "min", wantErr: true},
		{value: "abc", wantErr: true},
		{value: "5minutes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseDuration(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
