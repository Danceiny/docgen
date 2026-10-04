package route_test

import (
	"fmt"

	"github.com/Danceiny/docgen/route"
)

func ExampleResolve() {
	// The route of Get in the service "pet": the prefix, the service name and the
	// method name with its first letter lower-cased.
	fmt.Println(route.Resolve("pet", "Get", "", "/api"))

	// @path replaces the method name.
	fmt.Println(route.Resolve("pet", "Get", "/search", "/api"))

	// A service name may contain slashes, and an @path that repeats it is not
	// repeated in the route.
	fmt.Println(route.Resolve("store/order", "Place", "/store/order/place", "/api"))

	// Output:
	// /api/pet/get
	// /api/pet/search
	// /api/store/order/place
}

func ExampleSkipped() {
	// A method with "@apidoc: -" in its doc comment is neither routed nor documented.
	fmt.Println(route.Skipped([]string{"SetStore wires the service to its storage.", "", "@apidoc: -"}))
	fmt.Println(route.Skipped([]string{"Get returns one pet."}))

	// Output:
	// true
	// false
}
