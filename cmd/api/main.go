package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

var version = "1.0.0"

type Config struct {
	port int
	env  string
}

type application struct {
	config Config
	logger *slog.Logger
}

func main() {
	var cfg Config
	flag.IntVar(&cfg.port, "port", 4000, "Port address of the websocket server")
	flag.StringVar(&cfg.env, "environment", "development", "Environment (development|staging|production)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := application{
		config: cfg,
		logger: logger,
	}

	srv := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	logger.Info("Starting server", "addr", srv.Addr, "environment", cfg.env)
	err := srv.ListenAndServe()
	if err != nil {
		logger.Error(err.Error())
	}
	os.Exit(1)
}
