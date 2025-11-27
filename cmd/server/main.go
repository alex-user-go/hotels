package main

import (
	"log"
	"os"

	"github.com/alex-user-go/hotels/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
