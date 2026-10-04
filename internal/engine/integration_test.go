package engine

import (
	"reflect"
	"testing"
	"time"
)

// A time.Duration field with a default tag has the duration, in nanoseconds, as its example.
func TestDurationFieldDefaultTagIsParsed(t *testing.T) {
	type L1CacheConfig struct {
		DefaultTTL time.Duration `json:"defaultTTL" default:"10m"`
	}

	// get the type of the field
	structType := reflect.TypeOf(L1CacheConfig{})
	field, found := structType.FieldByName("DefaultTTL")
	if !found {
		t.Fatal("DefaultTTL field not found")
	}

	// get the value of the default tag
	defaultValue := field.Tag.Get("default")
	if defaultValue != "10m" {
		t.Fatalf("Expected default tag '10m', got '%s'", defaultValue)
	}

	// parse it
	result := parseExampleToInterface(defaultValue, field.Type)
	expected := int64(10 * time.Minute)

	if result != expected {
		t.Errorf("parseExampleToInterface() = %v (type: %T), expected %v (type: %T)",
			result, result, expected, expected)
	}

	t.Logf("Successfully parsed DefaultTTL default value '%s' as %v nanoseconds", defaultValue, result)
}

// TestAllDurationFormats tests the different duration formats.
func TestAllDurationFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"10m", 10 * time.Minute},
		{"5s", 5 * time.Second},
		{"1h", 1 * time.Hour},
		{"3m", 3 * time.Minute},
		{"30s", 30 * time.Second},
		{"1m30s", 1*time.Minute + 30*time.Second},
		{"2h30m", 2*time.Hour + 30*time.Minute},
	}

	durationType := reflect.TypeOf(time.Duration(0))

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseExampleToInterface(tt.input, durationType)
			expected := int64(tt.expected)

			if result != expected {
				t.Errorf("parseExampleToInterface('%s') = %v, expected %v", tt.input, result, expected)
			} else {
				t.Logf("✓ '%s' -> %v nanoseconds", tt.input, result)
			}
		})
	}
}
