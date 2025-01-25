package testhelpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrIfTrue(t *testing.T) {
	assert.Error(t, ErrIfTrue(true))
	assert.NoError(t, ErrIfTrue(false))
}

func TestErrOr1Val(t *testing.T) {
	testVal := "test"

	val, err := ErrOr1Val(testVal, true)
	assert.Empty(t, val)
	assert.Error(t, err)

	val, err = ErrOr1Val(testVal, false)
	assert.Equal(t, testVal, val)
	assert.NoError(t, err)
}

func TestErrExpected(t *testing.T) {
	tests := []struct {
		desc       string
		conditions []bool
		want       bool
	}{
		{
			desc:       "should return true if any condition is true",
			conditions: []bool{false, true, false},
			want:       true,
		},
		{
			desc:       "should return false if all conditions are false",
			conditions: []bool{false, false, false},
			want:       false,
		},
		{
			desc:       "should return false for empty conditions",
			conditions: []bool{},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			assert.Equal(t, tt.want, ErrExpected(tt.conditions...))
		})
	}
}

func TestValOrDefault(t *testing.T) {
	testVal := "test"
	defaultTestVal := "default"

	// Test with strings
	assert.Equal(t, testVal, ValOrDefault(testVal, defaultTestVal))
	assert.Equal(t, defaultTestVal, ValOrDefault("", defaultTestVal))
	assert.Equal(t, "", ValOrDefault("", ""))

	// Test with pointers
	assert.Equal(t, &testVal, ValOrDefault(&testVal, &defaultTestVal))
	assert.Equal(t, &defaultTestVal, ValOrDefault(nil, &defaultTestVal))
	assert.Equal(t, (*string)(nil), ValOrDefault[*string](nil, nil))
}
