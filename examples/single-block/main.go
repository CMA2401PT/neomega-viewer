// This example hashes and bakes a Java block without starting an HTTP server.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"neomega-blocks/ir"
	"neomega-viewer/singleblock"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "resource bundle directory")
	java := flag.String("java", "minecraft:stone []", "Java state in neomega-blocks IR syntax")
	seed := flag.Uint64("seed", 0, "deterministic model choice seed")
	flag.Parse()
	state := ir.IRBlock(*java).Tidy()
	engine, err := singleblock.Open(singleblock.OpenOptions{AssetsDir: *assets})
	if err != nil {
		log.Fatal(err)
	}
	block, err := engine.Build(state.Hash(), singleblock.Options{Seed: *seed})
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(block.Packet()); err != nil {
		log.Fatal(err)
	}
}
