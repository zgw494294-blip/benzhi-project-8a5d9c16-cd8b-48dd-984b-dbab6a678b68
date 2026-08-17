package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"example.com/baghold/internal/baghold"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "smoke" {
		if err := baghold.RunSmoke(os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}

	server := &http.Server{Addr: ":8080", Handler: baghold.NewHandler(baghold.NewStore())}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5_000_000_000)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	log.Printf("BagHold listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
