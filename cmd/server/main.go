package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"electra-assignment/internal/api"
	"electra-assignment/internal/service"
)

const defaultPort = "8080"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	server := http.Server{
		Addr:              ":" + port,
		Handler:           api.New(service.New(), logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting server", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
