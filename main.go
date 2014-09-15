package main

import (
	"flag"
	"os"
	"path/filepath"
	"log"
	"fmt"
	"github.com/garfunkel/nasimport/nasimporter"
)

func main() {
	defaultConfigPath := filepath.Join(filepath.Dir(os.Args[0]), "config.json")

	automaticMode := flag.Bool("a", false, "automatic mode (accept best content guess)")
	configPath := flag.String("c", defaultConfigPath, "config JSON file to read in")

	flag.Parse()

	importer, err := nasimporter.NewNasImporter(*configPath, *automaticMode)

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
