package mapped

// After uses Money after it is declared.
type After struct {
	// Price is the price.
	Price Money `json:"price"`
	Plain Money `json:"plain"`
}
