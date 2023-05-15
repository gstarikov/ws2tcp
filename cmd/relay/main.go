package main

import (
	"fmt"
	"github.com/gstarikov/ws2tcp/internal/relay"
	"github.com/gstarikov/ws2tcp/internal/start"
)

func main() {
	cfg := relay.Config{}

	ctx, stop, log := start.PrepareEnv(&cfg)
	defer stop()

	dev, err := relay.New(cfg, log)
	if err != nil {
		fmt.Printf("cant init device: %s\n", err)
		return
	}

	<-ctx.Done() // wait signal
	dev.WaitTillStop()
	log.Info("graceful shutdown")
}
