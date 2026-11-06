package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	renderresources "neomega-viewer-core/viewer/resources"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/viewassets"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	directory := flag.String("world", "", "Java/Bedrock 存档目录，或 schem/schematic/litematic/nbt/mcstructure/mcworld 文件")
	pack := flag.String("pack", "", "PackFS container with a Java world at its root")
	dimension := flag.String("dimension", "", "Java dimension directory or Bedrock dimension 0/1/2; default overworld")
	bedrockFix := flag.Bool("bedrock-fix", true, "repair Bedrock neighbour states after conversion")
	position := flag.String("position", "", "initial camera position: x,y,z")
	listen := flag.String("listen", "127.0.0.1:2412", "HTTP listen address")
	workers := flag.Int("workers", 4, "concurrent column builds (1..8)")
	structureMemory := flag.Int("structure-memory-mib", 512, "结构转换工作内存预算（MiB，至少 64）")
	structureWorkers := flag.Int("structure-workers", 0, "结构转 Java 的写出并发数上限，0 自动选择")
	browser := flag.Bool("browser", true, "启动后用默认浏览器打开页面")
	flag.Parse()
	if *directory != "" && *pack != "" {
		return fmt.Errorf("choose -world or -pack")
	}
	if *workers < 1 || *workers > 8 {
		return fmt.Errorf("-workers must be 1..8")
	}
	if *structureMemory < 64 || *structureWorkers < 0 {
		return fmt.Errorf("结构转换内存预算至少 64 MiB，workers 不能为负数")
	}
	options := SourceOptions{
		StructureMemoryMiB: *structureMemory,
		StructureWorkers:   *structureWorkers,
		Pack:               *pack,
		Dimension:          *dimension,
		SkipBedrockFix:     !*bedrockFix,
	}
	if *directory != "" {
		parsed, err := sourceAtPath(*directory)
		if err != nil {
			return err
		}
		parsed.Dimension = options.Dimension
		parsed.SkipBedrockFix = options.SkipBedrockFix
		parsed.StructureMemoryMiB = options.StructureMemoryMiB
		parsed.StructureWorkers = options.StructureWorkers
		options = parsed
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	source, err := OpenSource(ctx, options)
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
	assets, web := renderresources.Resources(), viewassets.Web()
	pipeline, err := newPipeline(resources)
	if err != nil {
		return err
	}
	worlds := NewWorlds(source, pipeline)
	owned = false
	defer worlds.Close()
	server := &http.Server{
		Addr: *listen,
		Handler: Handler(ServerOptions{
			StructureMemoryMiB: *structureMemory,
			StructureWorkers:   *structureWorkers,
			Worlds:             worlds,
			Assets:             assets,
			Web:                web,
			ResourceVersion:    resources.Metadata.Version,
			Workers:            *workers,
			SkipBedrockFix:     !*bedrockFix,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	// 先占住端口再印开场，网址出现时页面已经能打开。
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	address := browserAddress(listener.Addr())
	printBanner(os.Stdout, address, terminalColors())
	if *browser {
		openBrowser(address)
	}
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
