// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	_ "github.com/mattn/go-sqlite3"
)

// SQLStore implements a thread-safe storage for raw and json dumped payloads using SQLite
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore initializes a new payloads store with an SQLite DB
func NewSQLStore() *SQLStore {
	db, err := sql.Open("sqlite3", "./payloads.db")
	if err != nil {
		log.Fatal(err)
	}

	s := &SQLStore{
		db: db,
	}

	// To handle situation where the tables might not exist, attempt to create them
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS payloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		data BLOB NOT NULL,
		encoding VARCHAR(10) NOT NULL,
		route VARCHAR(20) NOT NULL
	);
	CREATE TABLE IF NOT EXISTS parsed_payloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		data TEXT NOT NULL,
		route VARCHAR(20) NOT NULL
	);
	`)

	if err != nil {
		log.Fatal("Failed to ensure table creation: ", err)
	}

	return s
}

// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
func (s *SQLStore) AppendPayload(route string, data []byte, encoding string, collectTime time.Time) error {
	_, err := s.db.Exec("INSERT INTO payloads (timestamp, data, encoding, route) VALUES (?, ?, ?, ?)", collectTime.Unix(), data, encoding, route)
	if err != nil {
		return err
	}

	count := 0
	row := s.db.QueryRow("SELECT COUNT(*) FROM payloads WHERE route = ?", route)
	if err := row.Scan(&count); err != nil {
		return err
	}

	rawPayload := api.Payload{
		Timestamp: collectTime,
		Data:      data,
		Encoding:  encoding,
	}

	return s.tryParseAndAppendPayload(rawPayload, route)
}

func (s *SQLStore) tryParseAndAppendPayload(rawPayload api.Payload, route string) error {
	parsedPayload, err := tryParse(rawPayload, route)
	if err != nil {
		return err
	}
	if parsedPayload == nil {
		return nil
	}

	data, err := json.Marshal(parsedPayload.Data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO parsed_payloads (timestamp, data, route) VALUES (?, ?, ?)", rawPayload.Timestamp.Unix(), data, route)
	if err != nil {
		return err
	}

	return nil
}

// CleanUpPayloadsOlderThan removes payloads older than specified time
func (s *SQLStore) CleanUpPayloadsOlderThan(time time.Time) {
	log.Printf("Cleaning up payloads")
	_, err := s.db.Exec("DELETE FROM payloads WHERE timestamp < ?", time.Unix())
	if err != nil {
		log.Println("Error cleaning payloads: ", err)
	}

	_, err = s.db.Exec("DELETE FROM parsed_payloads WHERE timestamp < ?", time.Unix())
	if err != nil {
		log.Println("Error cleaning parsed payloads: ", err)
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

		count := 0
		row := s.db.QueryRow("SELECT COUNT(*) FROM payloads WHERE route = ?", route)
		if err := row.Scan(&count); err != nil {
			log.Println("Error fetching payload count for route: ", route, " - ", err)
			continue
		}
	}
}

// GetRawPayloads returns all raw payloads for a given route
func (s *SQLStore) GetRawPayloads(route string) (payloads []api.Payload) {
	rows, err := s.db.Query("SELECT timestamp, data, encoding FROM payloads WHERE route = ?", route)
	if err != nil {
		log.Println("Error fetching raw payloads: ", err)
		return nil
	}
	defer rows.Close()

	var timestamp int64
	var data []byte
	var encoding string
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

// GetJSONPayloads returns all parsed payloads for a given route
func (s *SQLStore) GetJSONPayloads(route string) (payloads []api.ParsedPayload) {
	rows, err := s.db.Query("SELECT timestamp, data FROM parsed_payloads WHERE route = ?", route)
	if err != nil {
		log.Println("Error fetching parsed payloads: ", err)
		return nil
	}
	defer rows.Close()

	var timestamp int64
	var data string
	for rows.Next() {
		err := rows.Scan(&timestamp, &data)
		if err != nil {
			log.Println("Error scanning parsed payload: ", err)
			continue
		}
		payloads = append(payloads, api.ParsedPayload{
			Timestamp: time.Unix(timestamp, 0),
			Data:      data,
		})
	}
	return payloads
}

// GetRouteStats returns the number of payloads for each route
func (s *SQLStore) GetRouteStats() (statsByRoute map[string]int) {
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
func (s *SQLStore) Flush() {
	_, err := s.db.Exec("DELETE FROM payloads")
	if err != nil {
		log.Println("Error flushing payloads: ", err)
	}

	_, err = s.db.Exec("DELETE FROM parsed_payloads")
	if err != nil {
		log.Println("Error flushing parsed payloads: ", err)
	}
}
