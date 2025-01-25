package testhelpers

import (
	"github.com/stretchr/testify/assert"
)

// This are just minor helpers to clean up the test code a bit
// No major logic should be added to this package. Typically functions should
// be just a few lines long.

func ErrIfTrue(condition bool) error {
	if condition {
		return assert.AnError
	}
	return nil
}

func ErrOr1Val[T interface{}](val T, condition bool) (T, error) {
	if condition {
		var defaultVal T
		return defaultVal, assert.AnError
	}
	return val, nil
}

func ErrExpected(conditions ...bool) bool {
	for _, condition := range conditions {
		if condition {
			return true
		}
	}
	return false
}

func ValOrDefault[T comparable](val T, defaultVal T) T {
	var defaultValForComparison T

	if val == defaultValForComparison {
		return defaultVal
	}
	return val
}
