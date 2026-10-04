package engine

import (
	"reflect"
	"testing"
	"time"
)

func TestParseExampleToInterface_Duration(t *testing.T) {
	tests := []struct {
		name        string
		exampleStr  string
		fieldType   reflect.Type
		expected    interface{}
		description string
	}{
		{
			name:        "valid duration 10m",
			exampleStr:  "10m",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			expected:    int64(10 * time.Minute),
			description: "should parse 10m as 10 minutes in nanoseconds",
		},
		{
			name:        "valid duration 5s",
			exampleStr:  "5s",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			expected:    int64(5 * time.Second),
			description: "should parse 5s as 5 seconds in nanoseconds",
		},
		{
			name:        "valid duration 1h",
			exampleStr:  "1h",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			expected:    int64(1 * time.Hour),
			description: "should parse 1h as 1 hour in nanoseconds",
		},
		{
			name:        "invalid duration",
			exampleStr:  "invalid",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			expected:    "invalid",
			description: "should return original string for invalid duration",
		},
		{
			name:        "empty string",
			exampleStr:  "",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			expected:    nil,
			description: "should return nil for empty string",
		},
		{
			name:        "non-duration type",
			exampleStr:  "10m",
			fieldType:   reflect.TypeOf(""),
			expected:    "10m",
			description: "should return original string for non-duration types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseExampleToInterface(tt.exampleStr, tt.fieldType)
			if result != tt.expected {
				t.Errorf("parseExampleToInterface() = %v (type: %T), expected %v (type: %T)\n%s",
					result, result, tt.expected, tt.expected, tt.description)
			}
		})
	}
}

// TestDurationParsing tests the logic that parses a duration.
func TestDurationParsing(t *testing.T) {
	// test parsing "10m"
	duration, err := time.ParseDuration("10m")
	if err != nil {
		t.Fatalf("Failed to parse duration: %v", err)
	}

	expected := 10 * time.Minute
	if duration != expected {
		t.Errorf("Expected %v, got %v", expected, duration)
	}

	// check the number of nanoseconds
	nanoseconds := int64(duration)
	expectedNanos := int64(10 * time.Minute)
	if nanoseconds != expectedNanos {
		t.Errorf("Expected %d nanoseconds, got %d", expectedNanos, nanoseconds)
	}

	t.Logf("10m parsed as %v (%d nanoseconds)", duration, nanoseconds)
}
