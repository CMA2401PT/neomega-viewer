// hash 对应的几何从共享表取得。
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"neomega-blocks/ir"
	"neomega-viewer/demo/singleblock"
)

func main() {
	tables := flag.String("tables", "", "geometry table file; empty uses bundled tables")
	java := flag.String("java", "minecraft:stone []", "Java state in neomega-blocks IR syntax")
	flag.Parse()
	state := ir.IRBlock(*java).Tidy()
	engine, err := singleblock.Open(singleblock.OpenOptions{TablesPath: *tables})
	if err != nil {
		log.Fatal(err)
	}
	block, err := engine.Build(state.Hash())
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(block.Packet()); err != nil {
		log.Fatal(err)
	}
}
