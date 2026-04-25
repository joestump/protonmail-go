package protonmail_test

import (
	"fmt"
	"log"

	"github.com/joestump/protonmail-go"
)

// ExampleNewClient demonstrates the minimum incantation to construct a
// Client. WithAppVersion is the only required option; "Other" is the
// generic value the Proton web client itself sends for non-product apps.
func ExampleNewClient() {
	c, err := protonmail.NewClient(
		protonmail.WithAppVersion("Other"),
	)
	if err != nil {
		log.Fatal(err)
	}
	_ = c
	fmt.Println("client constructed")
	// Output: client constructed
}
