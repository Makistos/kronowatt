package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kronowatt/backend/internal/api"
	"kronowatt/backend/internal/config"
	"kronowatt/backend/internal/scheduler"
	"kronowatt/backend/internal/storage"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			runMigrate(os.Args[2:])
			return
		case "collect":
			runCollect(os.Args[2:])
			return
		case "seed":
			runSeed(os.Args[2:])
			return
		}
	}
	runServer()
}

func runServer() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := storage.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	fmiJob := newFMIJob(cfg, storage.NewWeatherRepository(pool), storage.NewCollectorRepository(pool))
	go scheduler.Run(ctx, "fmi_observation", cfg.FMIPollInterval, fmiJob)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.NewRouter(pool, cfg.DiskCheckPath)}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("kronowatt server listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
