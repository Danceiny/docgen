// Package snapshotmodels is a fixture package for the golden of ProcessModels
// schemas. Keep types small and stable — changing fields intentionally updates
// schemas.golden.json via UPDATE_SCHEMA_SNAPSHOT=1.
package snapshotmodels

// SnapshotStatus is a string alias used as a named schema component.
type SnapshotStatus string

const (
	SnapshotStatusPending SnapshotStatus = "pending"
	SnapshotStatusDone    SnapshotStatus = "done"
)

// SnapshotMeta is an embedded object for nested property coverage.
type SnapshotMeta struct {
	// Source identifies the fixture origin.
	Source string `json:"source" example:"fixture"`
}

// SnapshotOrder is the primary DTO locked by the ProcessModels golden.
type SnapshotOrder struct {
	ID     string         `json:"id"`
	Status SnapshotStatus `json:"status"`
	Amount *float64       `json:"amount,omitempty"`
	Tags   []string       `json:"tags"`
	Meta   SnapshotMeta   `json:"meta"`
}
