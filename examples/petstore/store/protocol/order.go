// Package protocol holds the requests and responses of the order service.
package protocol

// PlaceOrderReq asks to adopt a pet.
type PlaceOrderReq struct {
	PetID      string `json:"petId" validate:"required"`
	CustomerID string `json:"customerId" validate:"required"`
	// Note is a message to the shelter.
	Note string `json:"note,omitempty"`
}

// Order is an adoption that was placed.
type Order struct {
	ID         string `json:"id"`
	PetID      string `json:"petId"`
	CustomerID string `json:"customerId"`
	// Complete is true once the pet has left the shelter.
	Complete bool `json:"complete"`
}
