// Package config defines the application configuration structures.
//
// Separated from cmd to allow other packages (db, ssh, tui) to
// depend on config without importing Cobra.
package config

import "strconv"

// Config holds all application settings for an active connection.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string

	SSH SSHConfig
}

// SSHConfig holds SSH tunnel settings.
type SSHConfig struct {
	Enabled       bool
	Host          string
	Port          int
	User          string
	KeyPath       string
	KeyPassphrase string
}

// DSN builds a pgx-compatible connection string.
func (c Config) DSN() string {
	return "host=" + c.Host +
		" port=" + strconv.Itoa(c.Port) +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.Database +
		" sslmode=" + c.SSLMode
}

// FromConnection converts a saved Connection profile into a Config.
func FromConnection(conn Connection) Config {
	port, _ := strconv.Atoi(conn.Port)
	if port == 0 {
		port = 5432
	}
	sshPort, _ := strconv.Atoi(conn.SSH.Port)
	if sshPort == 0 {
		sshPort = 22
	}

	return Config{
		Host:     conn.Host,
		Port:     port,
		User:     conn.User,
		Password: conn.Password,
		Database: conn.Database,
		SSLMode:  conn.SSLMode,
		SSH: SSHConfig{
			Enabled:       conn.SSH.Enabled,
			Host:          conn.SSH.Host,
			Port:          sshPort,
			User:          conn.SSH.User,
			KeyPath:       conn.SSH.KeyPath,
			KeyPassphrase: conn.SSH.KeyPassphrase,
		},
	}
}
