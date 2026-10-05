// Package tagvalues is a fixture: the example and default tags of fields of many types.
package tagvalues

import "time"

// Code is a string type.
type Code string

// Level is a number type.
type Level int

// Thing has fields with examples and defaults.
type Thing struct {
	Timeout  *time.Duration `json:"timeout" example:"1h" default:"30s"`
	Plain    time.Duration  `json:"plain" example:"10m"`
	Pointer  *string        `json:"pointer" example:"123"`
	Custom   Code           `json:"custom" example:"456"`
	Level    Level          `json:"level" example:"3"`
	Big      int64          `json:"big" example:"1234567890123456789"`
	Ok       bool           `json:"ok" default:"true"`
	Ratio    float64        `json:"ratio" example:"0.5"`
	Names    []string       `json:"names" example:"[\"a\",\"b\"]"`
	BadCount int            `json:"badCount" example:"abc"`
	BadWhen  time.Time      `json:"badWhen" example:"2024-01-02"`
	BadFlag  bool           `json:"badFlag" default:"maybe"`
	Written  time.Duration  `json:"written" example:"30s" default:"5m"`
}
