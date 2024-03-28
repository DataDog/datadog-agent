// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"github.com/prometheus/client_golang/prometheus"
	// import sqlite3 driver
	_ "modernc.org/sqlite"
)

const (
	defaultDBPath = "payloads.db"

	metricsTicker = 30 * time.Second
)

// sqlStore implements a thread-safe storage for raw and json dumped payloads using SQLite
type sqlStore struct {
	db   *sql.DB
	path string

	stopCh  chan struct{}
	metrics sqlMetrics
}

type sqlMetrics struct {
	// nBPayloads is a prometheus metric to track the number of payloads collected by route
	nBPayloads *prometheus.GaugeVec
	// insertLatency is a prometheus metric to track the latency of inserting payloads
	insertLatency *prometheus.HistogramVec
	// ReadLatency is a prometheus metric to track the latency of reading payloads
	readLatency *prometheus.HistogramVec
	// diskUsage is a prometheus metric to track the disk usage of the store
	diskUsage *prometheus.GaugeVec
}

// newSQLStore initializes a new payloads store with an SQLite DB
func newSQLStore() *sqlStore {
	p := os.Getenv("SQLITE_DB_PATH")
	if p == "" {
		f, err := os.CreateTemp("", defaultDBPath)
		if err != nil {
			log.Fatal(err)
		}
		p = f.Name()
	}
	db, err := sql.Open("sqlite", p)
	if err != nil {
		log.Fatal(err)
	}

	// Enable WAL mode for better performances and limit the heap
	_, err = db.Exec(`
	PRAGMA journal_mode=WAL;
	PRAGMA soft_heap_limit=100000000;
	`)
	if err != nil {
		log.Fatal("Failed to enable WAL mode: ", err)
	}

	s := &sqlStore{
		path:   p,
		db:     db,
		stopCh: make(chan struct{}),

		metrics: sqlMetrics{
			nBPayloads: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "payloads",
				Help: "Number of payloads collected by route",
			}, []string{"route"}),
			insertLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "insert_latency",
				Help:    "Latency of inserting payloads",
				Buckets: prometheus.DefBuckets,
			}, []string{"route"}),
			readLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "read_latency",
				Help:    "Latency of reading payloads",
				Buckets: prometheus.DefBuckets,
			}, []string{"route"}),
			diskUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "disk_usage",
				Help: "Disk usage of the store",
			}, []string{"route"}),
		},
	}

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS payloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		data BLOB NOT NULL,
		encoding VARCHAR(10) NOT NULL,
		route VARCHAR(20) NOT NULL
	);
	`)

	if err != nil {
		log.Fatal("Failed to ensure table creation: ", err)
	}

	s.startMetricsCollector()
	return s
}

// Close closes the store
func (s *sqlStore) Close() {
	s.db.Close()
	s.stopCh <- struct{}{}
	os.Remove(s.path)
}

// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
func (s *sqlStore) AppendPayload(route string, data []byte, encoding string, collectTime time.Time) error {
	now := time.Now()
	_, err := s.db.Exec("INSERT INTO payloads (timestamp, data, encoding, route) VALUES (?, ?, ?, ?)", collectTime.Unix(), data, encoding, route)
	if err != nil {
		return err
	}
	obs := time.Since(now).Seconds()
	s.metrics.insertLatency.WithLabelValues(route).Observe(obs)
	log.Printf("Inserted payload for route %s in %f seconds\n", route, obs)

	return err
}

// CleanUpPayloadsOlderThan removes payloads older than specified time
func (s *sqlStore) CleanUpPayloadsOlderThan(time time.Time) {
	log.Printf("Cleaning up payloads")
	_, err := s.db.Exec("DELETE FROM payloads WHERE timestamp < ?", time.Unix())
	if err != nil {
		log.Println("Error cleaning payloads: ", err)
	}

	routes, err := s.db.Query("SELECT DISTINCT route FROM payloads")
	if err != nil {
		log.Println("Error fetching distinct routes: ", err)
		return
	}
	defer routes.Close()

	for routes.Next() {
		var route string
		if err := routes.Scan(&route); err != nil {
			log.Println("Error scanning route: ", err)
			continue
		}
	}
}

// GetRawPayloads returns all raw payloads for a given route
func (s *sqlStore) GetRawPayloads(route string) []api.Payload {
	now := time.Now()
	rows, err := s.db.Query("SELECT timestamp, data, encoding FROM payloads WHERE route = ?", route)
	if err != nil {
		log.Println("Error fetching raw payloads: ", err)
		return nil
	}
	defer rows.Close()
	s.metrics.readLatency.WithLabelValues(route).Observe(time.Since(now).Seconds())

	var timestamp int64
	var data []byte
	var encoding string
	payloads := []api.Payload{}
	for rows.Next() {
		err := rows.Scan(&timestamp, &data, &encoding)
		if err != nil {
			log.Println("Error scanning raw payload: ", err)
			continue
		}
		payloads = append(payloads, api.Payload{
			Timestamp: time.Unix(timestamp, 0),
			Data:      data,
			Encoding:  encoding,
		})
	}
	return payloads
}

// GetRouteStats returns the number of payloads for each route
func (s *sqlStore) GetRouteStats() (statsByRoute map[string]int) {
	statsByRoute = make(map[string]int)
	rows, err := s.db.Query("SELECT route, COUNT(*) FROM payloads GROUP BY route")
	if err != nil {
		log.Println("Error fetching route stats: ", err)
		return
	}
	defer rows.Close()

	var route string
	var count int
	for rows.Next() {
		err := rows.Scan(&route, &count)
		if err != nil {
			log.Println("Error scanning route stat: ", err)
			continue
		}
		statsByRoute[route] = count
	}
	return statsByRoute
}

// Flush flushes the store
func (s *sqlStore) Flush() {
	_, err := s.db.Exec("DELETE FROM payloads")
	if err != nil {
		log.Println("Error flushing payloads: ", err)
	}
}

// GetInternalMetrics returns the prometheus metrics for the store
func (s *sqlStore) GetInternalMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		s.metrics.nBPayloads,
		s.metrics.insertLatency,
		s.metrics.readLatency,
		s.metrics.diskUsage,
	}
}

func (s *sqlStore) startMetricsCollector() {
	go func() {
		ticker := time.NewTicker(metricsTicker)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.collectDiskUsage()
				s.collectPayloads()
			}
		}
	}()
}

func (s *sqlStore) collectDiskUsage() {
	// update disk usage
	var diskUsage int
	err := s.db.QueryRow("SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&diskUsage)
	if err != nil {
		log.Println("Error fetching disk usage: ", err)
		return
	}
	s.metrics.diskUsage.WithLabelValues("all").Set(float64(diskUsage))
}

func (s *sqlStore) collectPayloads() {
	routes, err := s.db.Query("SELECT route, COUNT(*) FROM payloads GROUP BY route")
	if err != nil {
		log.Println("Error fetching route stats: ", err)
		return
	}
	defer routes.Close()
	for routes.Next() {
		var route string
		var count int
		if err := routes.Scan(&route, &count); err != nil {
			log.Println("Error scanning route stat: ", err)
			continue
		}
		s.metrics.nBPayloads.WithLabelValues(route).Set(float64(count))
	}
}
