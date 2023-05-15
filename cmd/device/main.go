package main

import (
	"fmt"
	"github.com/gstarikov/ws2tcp/internal/device"
	"github.com/gstarikov/ws2tcp/internal/start"
)

func main() {
	cfg := device.Config{}

	ctx, stop, log := start.PrepareEnv(&cfg)
	defer stop()

	dev, err := device.New(ctx, cfg, log)
	if err != nil {
		fmt.Printf("cant init device: %s\n", err)
		return
	}

	dev.WaitTillStop()
	log.Info("graceful shutdown")
}
