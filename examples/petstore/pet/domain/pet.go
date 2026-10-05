// Package domain holds the types the pet service stores and returns.
package domain

import "time"

// Status is where a pet is in the adoption process.
//
// Old clients may still send the retired values, which the service accepts; the
// directive below leaves them out of the documents.
//
//apidoc:public:-StatusLegacy*
type Status string

const (
	// StatusAvailable means the pet can be adopted.
	StatusAvailable Status = "available"
	// StatusReserved means the pet is held for a customer.
	StatusReserved Status = "reserved"
	// StatusAdopted means the pet has a home.
	StatusAdopted Status = "adopted"
	// StatusLegacyQuarantine was replaced by a flag on the pet.
	StatusLegacyQuarantine Status = "quarantine"
)

// Size is how big a pet grows. The values are what iota counts, and the document
// has them.
type Size int

const (
	// SizeSmall means up to 10 kg.
	SizeSmall Size = iota
	// SizeMedium means up to 25 kg.
	SizeMedium
	// SizeLarge means more than that.
	SizeLarge
)

// Category groups pets: dogs, cats, birds.
type Category struct {
	ID   int64  `json:"id" example:"7"`
	Name string `json:"name" example:"dog"`
	// Children are the more specific categories: dogs have breeds. A type that
	// contains itself is described once and referred to by name.
	Children []*Category `json:"children,omitempty"`
}

// Price is an amount of money in dollars. It is written as a string such as
// "12.50", not as a number, so that no client rounds it.
type Price int64

// Pet is an animal that can be adopted.
type Pet struct {
	// ID is assigned when the pet is created.
	ID string `json:"id" example:"p-1001" validate:"required"`
	// Name is what the pet is called.
	Name     string   `json:"name" example:"Rex" validate:"required"`
	Status   Status   `json:"status"`
	Category Category `json:"category"`
	// Tags are free words that help to find the pet.
	Tags []string `json:"tags,omitempty"`
	// Attributes are free-form labels, such as the colour; the values are text.
	Attributes map[string]string `json:"attributes,omitempty"`
	Size       Size              `json:"size"`
	BornAt     time.Time         `json:"bornAt"`
	// Fee is the adoption fee.
	Fee Price `json:"fee"`
	// Notes are written by the staff and never shown to customers.
	Notes string `json:"notes" apidoc:"internal"`
	// VetCost is what the pet cost until now; no document shows it.
	VetCost *float64 `json:"vetCost,omitempty" apidoc:"hidden"`
}

// InternalAuditEntry records who changed a pet. A type named Internal* stays out
// of the public document (docgen.yaml: hide_type_prefixes).
type InternalAuditEntry struct {
	PetID string `json:"petId"`
	By    string `json:"by"`
}
