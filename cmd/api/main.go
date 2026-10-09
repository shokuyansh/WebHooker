package main

import (
	"context"
	"database/sql"
	"flag"
	"log/slog"
	"math"
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
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
}

type application struct {
	config       Config
	logger       *slog.Logger
	models       data.Models
	wg           sync.WaitGroup
	cancelWorker context.CancelFunc
}

func main() {
	var cfg Config
	flag.IntVar(&cfg.port, "port", 4000, "Port address of the websocket server")
	flag.StringVar(&cfg.env, "environment", "development", "Environment (development|staging|production)")

	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("WEBHOOKER_DB_DSN"), "PostgresSQL DSN ")
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "POSTGRES SQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "POSTGRES SQL max idle connections")
	flag.DurationVar(&cfg.db.maxIdleConnTime, "db-max-idle-time", 15*time.Minute, "POSTGRES SQL max connection idle time")

	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if cfg.limiter.enabled {
		if cfg.limiter.rps <= 0 ||
			math.IsNaN(cfg.limiter.rps) ||
			math.IsInf(cfg.limiter.rps, 0) {
			logger.Error("limiter-rps must be finite and greater than zero")
			os.Exit(1)
		}

		if cfg.limiter.burst < 1 {
			logger.Error("limiter-burst must be at least 1")
			os.Exit(1)
		}
	}

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
	app.cancelWorker = stopWorker

	defer func() {
		stopWorker()
		app.wg.Wait()
	}()

	app.background(func() {
		app.runDeliveryWorker(workerCtx)
	})
	err = app.serve()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
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
