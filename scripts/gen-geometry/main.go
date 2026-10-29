package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"os"

	"neomega-viewer/scripts/internal/tablegen"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "matching resource bundle")
	output := flag.String("output", "resources/geometry.gob", "output geometry table")
	flag.Parse()
	if err := generate(*assets, *output); err != nil {
		log.Fatal(err)
	}
}

func generate(assetsDir, output string) error {
	builder, err := tablegen.Open(tablegen.OpenOptions{AssetsDir: assetsDir})
	if err != nil {
		return err
	}
	tables, err := builder.Build()
	if err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(file).Encode(tables); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	fmt.Printf("%s: indexed=%d shapes=%d skipped=%d\n", output, len(tables.Index), len(tables.Shapes), len(tables.Skipped))
	return nil
}
