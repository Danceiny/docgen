package autowire

// Late is a union that is declared after the struct that uses it.
//
// @autowire: true
type Late interface {
	isLate()
}

// Gamma is an alternative of Late.
type Gamma struct {
	G string `json:"g"`
}

func (Gamma) isLate() {}

// Delta is an alternative of Late.
type Delta struct {
	D int `json:"d"`
}

func (*Delta) isLate() {}

// Lonely is a union that nothing implements.
//
// @autowire: true
type Lonely interface {
	isLonely()
}
