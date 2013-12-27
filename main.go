package main

import (
	"flag"
	"github.com/garfunkel/nasimport/nasimporter"
)

func main() {
	flag.Parse()

	importer := nasimporter.NewNasImporter()

	for _, path := range flag.Args() {
		importer.Import(path)
	}
}
