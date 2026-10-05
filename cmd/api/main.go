package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shokuyansh/Webhooker/internal/data"
)

var version = "1.0.0"

type Config struct {
	port int
	env  string
	db   struct {
		dsn             string
		maxOpenConns    int
		maxIdleConns    int
		maxIdleConnTime time.Duration
	}
}

type application struct {
	config Config
	logger *slog.Logger
	models data.Models
	wg     sync.WaitGroup
}

func main() {
	var cfg Config
	flag.IntVar(&cfg.port, "port", 4000, "Port address of the websocket server")
	flag.StringVar(&cfg.env, "environment", "development", "Environment (development|staging|production)")
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("WEBHOOKER_DB_DSN"), "PostgresSQL DSN ")
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "POSTGRES SQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "POSTGRES SQL max idle connections")
	flag.DurationVar(&cfg.db.maxIdleConnTime, "db-max-idle-time", 15*time.Minute, "POSTGRES SQL max connection idle time")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := openDB(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database connection pool established")
	app := application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
	}

	workerCtx, stopWorker := context.WithCancel(context.Background())

	defer func() {
		stopWorker()
		app.wg.Wait()
	}()

	app.background(func() {
		app.runDeliveryWorker(workerCtx)
	})

	srv := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	logger.Info("Starting server", "addr", srv.Addr, "environment", cfg.env)
	err = srv.ListenAndServe()
	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func openDB(cfg Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)
	db.SetConnMaxIdleTime(cfg.db.maxIdleConnTime)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
