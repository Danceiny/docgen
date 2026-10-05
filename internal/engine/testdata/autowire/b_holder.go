package autowire

// Holder uses unions, each twice, so that one of the uses comes after the
// declaration of the union and before the alternatives are added.
type Holder struct {
	First  Early  `json:"first"`
	Second Early  `json:"second"`
	Third  Late   `json:"third"`
	Fourth Late   `json:"fourth"`
	Fifth  Lonely `json:"fifth"`
}
