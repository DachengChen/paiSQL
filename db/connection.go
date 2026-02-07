// Package db manages the pgx PostgreSQL connection pool.
//
// Design decisions:
//   - Uses pgxpool for connection pooling (safe for concurrent access).
//   - All queries are executed through the Pool interface, keeping the
//     rest of the application unaware of connection details.
//   - SSH tunnel integration is handled transparently: if SSH is enabled,
//     we first establish the tunnel, then connect pgx to the local endpoint.
package db

import (
	"context"
	"fmt"

	"github.com/DachengChen/paiSQL/config"
	"github.com/DachengChen/paiSQL/ssh"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgx connection pool and optional SSH tunnel.
type DB struct {
	Pool   *pgxpool.Pool
	Tunnel *ssh.Tunnel
}

// Connect establishes a PostgreSQL connection, optionally through an SSH tunnel.
func Connect(ctx context.Context, cfg config.Config) (*DB, error) {
	d := &DB{}

	// If SSH tunnel is requested, set it up first.
	if cfg.SSH.Enabled {
		tunnel, err := ssh.NewTunnel(cfg.SSH, cfg.Host, cfg.Port)
		if err != nil {
			return nil, fmt.Errorf("ssh tunnel: %w", err)
		}
		localAddr, err := tunnel.Start(ctx)
		if err != nil {
			return nil, fmt.Errorf("ssh tunnel start: %w", err)
		}
		d.Tunnel = tunnel

		// Override connection target with local tunnel endpoint
		cfg.Host = localAddr.Host
		cfg.Port = localAddr.Port
	}

	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("pgx connect: %w", err)
	}

	// Verify the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgx ping: %w", err)
	}

	d.Pool = pool
	return d, nil
}

// Close shuts down the pool and SSH tunnel.
func (d *DB) Close() {
	if d.Pool != nil {
		d.Pool.Close()
	}
	if d.Tunnel != nil {
		d.Tunnel.Stop()
	}
}
