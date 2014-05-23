package main

import (
	"flag"
	"log"
	"fmt"
	"github.com/garfunkel/nasimport/nasimporter"
)

func main() {
	automaticMode := flag.Bool("a", false, "automatic mode (accept best content guess)")

	flag.Parse()

	importer, err := nasimporter.NewNasImporter("config.json", *automaticMode)

	if err != nil {
		log.Fatal(err)
	}

	numImported := 0

	for _, path := range flag.Args() {
		err = importer.Import(path)

		if err != nil {
			log.Fatal(err)
		} else {
			numImported++
		}
	}

	fmt.Printf("%v files imported.\n", numImported)
}
