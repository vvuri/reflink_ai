// Command reflink runs the RefLink web service: an MVP image inspector that
// reports a photo's basic properties, camera EXIF data and GPS coordinates,
// never storing the uploaded file.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reflink/internal/appcfg"
	"reflink/internal/server"
	"reflink/internal/uploadlog"
)

func main() {
	cfg, err := appcfg.Load()
	if err != nil {
		log.Fatalf("reflink: config: %v", err)
	}

	uplog, err := uploadlog.New(cfg.LogFile)
	if err != nil {
		log.Fatalf("reflink: upload log: %v", err)
	}
	defer uplog.Close()

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           server.New(cfg, uplog).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Print("reflink: shutting down")
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shCtx); err != nil {
			log.Printf("reflink: shutdown: %v", err)
		}
	}()

	log.Printf("reflink: listening on http://0.0.0.0:%d", cfg.Port)
	log.Printf("reflink: max upload %d MB, upload log %q", cfg.MaxUploadMB, cfg.LogFile)

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("reflink: %v", err)
	}
}
