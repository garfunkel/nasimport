package main

import (
	"flag"
	"./nasimporter"
)

func main() {
	flag.Parse()

	importer := nasimporter.NewNasImporter()

	for _, path := range flag.Args() {
		importer.Import(path)
	}
}
