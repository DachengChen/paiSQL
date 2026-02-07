// connections.go manages saved database connections.
//
// Connections are stored in ~/.paisql/connections.yaml so users
// can quickly reconnect without retyping credentials.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Connection is a named, saveable database connection profile.
type Connection struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     string   `json:"port"`
	User     string   `json:"user"`
	Password string   `json:"password"`
	Database string   `json:"database"`
	SSLMode  string   `json:"ssl_mode"`
	SSH      SSHEntry `json:"ssh,omitempty"`
}

// SSHEntry holds SSH tunnel settings for a saved connection.
type SSHEntry struct {
	Enabled       bool   `json:"enabled,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          string `json:"port,omitempty"`
	User          string `json:"user,omitempty"`
	KeyPath       string `json:"key_path,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`
}

// ConnectionStore manages saved connections on disk.
type ConnectionStore struct {
	path        string
	Connections []Connection `json:"connections"`
}

// NewConnectionStore creates a store, loading from ~/.paisql/connections.json.
func NewConnectionStore() (*ConnectionStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(homeDir, ".paisql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	store := &ConnectionStore{
		path: filepath.Join(dir, "connections.json"),
	}

	// Load existing connections
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("parse connections: %w", err)
	}

	return store, nil
}

// Save writes all connections to disk.
func (s *ConnectionStore) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// Add adds or updates a connection by name.
func (s *ConnectionStore) Add(conn Connection) {
	for i, c := range s.Connections {
		if c.Name == conn.Name {
			s.Connections[i] = conn
			return
		}
	}
	s.Connections = append(s.Connections, conn)
}

// Delete removes a connection by name.
func (s *ConnectionStore) Delete(name string) {
	for i, c := range s.Connections {
		if c.Name == name {
			s.Connections = append(s.Connections[:i], s.Connections[i+1:]...)
			return
		}
	}
}

// Get retrieves a connection by name.
func (s *ConnectionStore) Get(name string) (Connection, bool) {
	for _, c := range s.Connections {
		if c.Name == name {
			return c, true
		}
	}
	return Connection{}, false
}

// DefaultConnection returns a connection with sensible defaults.
func DefaultConnection() Connection {
	return Connection{
		Name:     "",
		Host:     "localhost",
		Port:     "5432",
		User:     "postgres",
		Password: "",
		Database: "postgres",
		SSLMode:  "disable",
		SSH: SSHEntry{
			Port: "22",
		},
	}
}
