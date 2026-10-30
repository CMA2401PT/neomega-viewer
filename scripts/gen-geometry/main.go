package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"neomega-viewer/scripts/internal/tablegen"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "matching resource bundle")
	output := flag.String("output", "resources/geometry.nvtables", "output geometry table")
	solid := flag.String("solid", "../neomega-assets/solidtable/geometry.nvsolid", "output solid table, embedded by neomega-assets/solidtable")
	flag.Parse()
	if err := generate(*assets, *output, *solid); err != nil {
		log.Fatal(err)
	}
}

func generate(assetsDir, output, solidPath string) error {
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
	if err := tables.Encode(file); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	solidFile, err := os.Create(solidPath)
	if err != nil {
		return err
	}
	if err := tables.Solid.Encode(solidFile); err != nil {
		solidFile.Close()
		return err
	}
	if err := solidFile.Close(); err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(tables.Metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(output+".java.json", append(metadata, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("%s: indexed=%d shapes=%d skipped=%d\n", output, len(tables.Index), len(tables.Shapes), len(tables.Skipped))
	fmt.Printf("%s: entries=%d\n", solidPath, len(tables.Solid))
	return nil
}
