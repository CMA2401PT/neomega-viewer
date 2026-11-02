package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"celler/java"
	jmodel "neomega-blocks/geometry/java_block_model"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/viewassets"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	directory := flag.String("world", "", "Java world directory; omit for an empty scene")
	pack := flag.String("pack", "", "PackFS container with a Java world at its root")
	dimension := flag.String("dimension", "", "relative dimension directory; default detects overworld")
	position := flag.String("position", "", "initial camera position: x,y,z")
	listen := flag.String("listen", "127.0.0.1:2412", "HTTP listen address")
	workers := flag.Int("workers", 4, "concurrent column builds (1..8)")
	flag.Parse()
	if *workers < 1 || *workers > 8 {
		return fmt.Errorf("-workers must be 1..8")
	}
	options := SourceOptions{
		Directory: *directory,
		Pack:      *pack,
		Dimension: java.Dimension(*dimension),
	}
	if *position != "" {
		parts := strings.Split(*position, ",")
		if len(parts) != 3 {
			return fmt.Errorf("-position requires x,y,z")
		}
		var p [3]float64
		for i, value := range parts {
			v, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("invalid -position")
			}
			p[i] = v
		}
		if math.Abs(p[0]) >= 30000000 || math.Abs(p[2]) >= 30000000 || p[1] < -2048 || p[1] >= 2048 {
			return fmt.Errorf("-position outside supported bounds")
		}
		options.Position = &p
	}
	source, err := OpenSource(options)
	if err != nil {
		return err
	}
	owned := true
	defer func() {
		if owned {
			source.Close()
		}
	}()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		return err
	}
	assets, web := viewassets.Resources(), viewassets.Web()
	data, err := fs.ReadFile(assets, "blocksAtlases.json")
	if err != nil {
		return err
	}
	atlas := &jmodel.Atlas{}
	if err = json.Unmarshal(data, atlas); err != nil {
		return err
	}
	pipeline, err := newPipeline(resources.Tables, atlas)
	if err != nil {
		return err
	}
	worlds := NewWorlds(source, pipeline)
	owned = false
	defer worlds.Close()
	server := &http.Server{
		Addr: *listen,
		Handler: Handler(ServerOptions{
			Worlds:          worlds,
			Assets:          assets,
			Web:             web,
			ResourceVersion: resources.Metadata.Version,
			Workers:         *workers,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Printf("http://%s · world=%q · dimension=%s", *listen, source.Info.Name, source.Info.Dimension)
	select {
	case err := <-done:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
