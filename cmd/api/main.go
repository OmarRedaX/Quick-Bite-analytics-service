// Command api is the process entry point. It does one thing: hand control
// to lib/boot and exit with its error. All wiring lives in lib/boot/boot.go
// — adding a new singleton means editing Run(), never this file.
package main

import (
	"log"

	"analytics-service/lib/boot"
)

func main() {
	if err := boot.Run(); err != nil {
		log.Fatal(err)
	}
}
