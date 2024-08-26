package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"

	_ "github.com/lib/pq"
)

type config struct {
	hostname string
	port     int
	username string
	password string
	database string
	table    string
}

func main() {
	var hostname, username, password, database, table string
	var workers, port int

	flag.StringVar(&hostname, "h", "127.0.0.1", "Host")
	flag.StringVar(&username, "u", "user", "Username")
	flag.StringVar(&password, "p", "pass", "Password")
	flag.StringVar(&database, "d", "db", "Database")
	flag.StringVar(&table, "t", "table", "Table")

	flag.IntVar(&workers, "w", 1, "Workers")
	flag.IntVar(&port, "P", 5432, "Port")

	flag.Parse()

	cfg := config{
		hostname: hostname,
		port:     port,
		username: username,
		password: password,
		database: database,
		table:    table,
	}

	if err := initalizeDB(context.Background(), cfg); err != nil {
		log.Fatalf("failed to initialize db: %v", err)
	}

	var wg sync.WaitGroup
	cancellableCtx, cancelFunc := context.WithCancel(context.Background())

	for i := 0; i < workers; i++ {
		wg.Add(1)
		id := i

		go func() {
			defer wg.Done()
			writer(cancellableCtx, id, cfg)
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	sig := <-c
	log.Printf("Received %+v", sig)
	cancelFunc()

	wg.Wait()
}

func initalizeDB(ctx context.Context, cfg config) error {
	connStr := fmt.Sprintf("user=%s password='%s' dbname=postgres host=%s port=%d", cfg.username, cfg.password, cfg.hostname, cfg.port)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to db: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping: %w", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.database))
	if err != nil {
		log.Printf("failed to create database: %v", err)
	}

	connStr = fmt.Sprintf("user=%s password='%s' dbname=%s host=%s port=%d", cfg.username, cfg.password, cfg.database, cfg.hostname, cfg.port)
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to db: %w", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id SERIAL PRIMARY KEY, a VARCHAR(256), b VARCHAR(256), c VARCHAR(256), d VARCHAR(256), e VARCHAR(256))", cfg.table))
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func writer(ctx context.Context, id int, cfg config) {
	for {
		if errors.Is(ctx.Err(), context.Canceled) {
			log.Printf("worker %d: shutting down", id)
			return
		}

		connStr := fmt.Sprintf("user=%s password='%s' dbname=%s host=%s port=%d", cfg.username, cfg.password, cfg.database, cfg.hostname, cfg.port)
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			log.Fatal(err)
		}

		if err := db.PingContext(ctx); err != nil {
			log.Printf("worker %d: failed to ping: %v", id, err)
			db.Close()
			continue
		}

		_, err = db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (a, b, c, d, e) VALUES ($1, $2, $3, $4, $5)", cfg.table), randSeq(256), randSeq(256), randSeq(256), randSeq(256), randSeq(256))
		if err != nil {
			log.Printf("worker %d: failed to insert: %v\n", id, err)
			db.Close()
			continue
		}

		log.Printf("worker %d: write succeededs", id)

		if err := db.Close(); err != nil {
			log.Printf("worker %d: failed to disconnect: %v", id, err)
		}
	}

}
