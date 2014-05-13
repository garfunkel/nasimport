package main

import (
	"flag"
	"log"
	"github.com/garfunkel/nasimport/nasimporter"
)

func main() {
	flag.Parse()

	importer, err := nasimporter.NewNasImporter("config.json")

	if err != nil {
		log.Fatal(err)
	}

	for _, path := range flag.Args() {
		err = importer.Import(path)

		if err != nil {
			log.Fatal(err)
		}
	}
}
