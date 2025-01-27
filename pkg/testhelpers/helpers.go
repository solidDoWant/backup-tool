package testhelpers

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/fatih/structtag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type runnableTB interface {
	testing.TB
	Run(name string, f func(t *testing.T)) bool
}

func OptStructTest[T interface{}](t runnableTB) {
	var defaultVal T
	reflectType := reflect.TypeOf(defaultVal)

	fieldCount := reflectType.NumField()
	for fieldNum := range fieldCount {
		field := reflectType.Field(fieldNum)
		if !field.IsExported() {
			continue
		}

		t.Run(
			fmt.Sprintf("Test %s field", field.Name),
			func(t *testing.T) {
				tags, err := structtag.Parse(string(field.Tag))
				require.NoError(t, err)

				// YAML name must be set using YAML convention (camel case)
				assert.Contains(t, tags.Keys(), "yaml")
				yamlTag, err := tags.Get("yaml")
				require.NoError(t, err)
				if yamlTag.HasOption("inline") {
					assert.Empty(t, yamlTag.Name)
				} else {
					// Check for snake case
					assert.NotEmpty(t, yamlTag.Name)
					assert.Regexp(t, "^[a-z]+[A-Za-z]*$", yamlTag.Name)
				}

				// Option structs cannot have required fields
				if slices.Contains(tags.Keys(), "jsonschema") {
					jsonSchemaTag, err := tags.Get("jsonschema")
					require.NoError(t, err)
					assert.NotEqual(t, "required", jsonSchemaTag.Name)
					assert.NotContains(t, jsonSchemaTag.Options, "required")
				}
			},
		)
	}
}
