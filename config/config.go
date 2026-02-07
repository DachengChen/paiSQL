// Package config defines the application configuration structures.
//
// Separated from cmd to allow other packages (db, ssh, tui) to
// depend on config without importing Cobra.
package config

// Config holds all application settings.
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
// When SSH tunnel is active, the caller should override Host/Port
// with the local tunnel endpoint.
func (c Config) DSN() string {
	return "host=" + c.Host +
		" port=" + itoa(c.Port) +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.Database +
		" sslmode=" + c.SSLMode
}

// itoa is a simple int-to-string without importing strconv at top-level.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
