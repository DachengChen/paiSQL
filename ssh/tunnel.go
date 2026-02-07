// Package ssh implements SSH local port forwarding for accessing
// remote PostgreSQL servers through a bastion/jump host.
//
// Design decisions:
//   - Uses golang.org/x/crypto/ssh for the SSH client.
//   - Allocates a random local port (":0") to avoid conflicts.
//   - The tunnel runs in a background goroutine and is stopped
//     via the Stop method (which closes the listener).
//   - Only key-based authentication is supported (with optional passphrase).
package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/DachengChen/paiSQL/config"
	"golang.org/x/crypto/ssh"
)

// Addr represents host:port of the local tunnel endpoint.
type Addr struct {
	Host string
	Port int
}

// Tunnel manages an SSH local port forward.
type Tunnel struct {
	sshConfig  *ssh.ClientConfig
	sshAddr    string // e.g. "bastion:22"
	remoteAddr string // e.g. "db-host:5432"

	client   *ssh.Client
	listener net.Listener
	wg       sync.WaitGroup
	done     chan struct{}
}

// NewTunnel creates a tunnel configuration (does not connect yet).
func NewTunnel(cfg config.SSHConfig, pgHost string, pgPort int) (*Tunnel, error) {
	authMethods, err := buildAuthMethods(cfg)
	if err != nil {
		return nil, err
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: proper host key verification
	}

	return &Tunnel{
		sshConfig:  sshConfig,
		sshAddr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		remoteAddr: net.JoinHostPort(pgHost, strconv.Itoa(pgPort)),
		done:       make(chan struct{}),
	}, nil
}

// Start opens the SSH connection and starts forwarding.
// Returns the local address to connect pgx to.
func (t *Tunnel) Start(ctx context.Context) (*Addr, error) {
	var err error
	t.client, err = ssh.Dial("tcp", t.sshAddr, t.sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", t.sshAddr, err)
	}

	// Listen on random local port
	t.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.client.Close()
		return nil, fmt.Errorf("local listen: %w", err)
	}

	// Extract the assigned port
	tcpAddr := t.listener.Addr().(*net.TCPAddr)
	localAddr := &Addr{Host: "127.0.0.1", Port: tcpAddr.Port}

	// Accept connections in background
	t.wg.Add(1)
	go t.acceptLoop()

	return localAddr, nil
}

// Stop tears down the tunnel.
func (t *Tunnel) Stop() {
	close(t.done)
	if t.listener != nil {
		t.listener.Close()
	}
	t.wg.Wait()
	if t.client != nil {
		t.client.Close()
	}
}

// acceptLoop accepts local connections and forwards them through SSH.
func (t *Tunnel) acceptLoop() {
	defer t.wg.Done()
	for {
		localConn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.done:
				return
			default:
				continue
			}
		}
		t.wg.Add(1)
		go t.forward(localConn)
	}
}

// forward pipes data between local and remote connections.
func (t *Tunnel) forward(localConn net.Conn) {
	defer t.wg.Done()
	defer localConn.Close()

	remoteConn, err := t.client.Dial("tcp", t.remoteAddr)
	if err != nil {
		return
	}
	defer remoteConn.Close()

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()
	<-done
}

// buildAuthMethods creates SSH auth methods from config.
func buildAuthMethods(cfg config.SSHConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if cfg.KeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key %s: %w", cfg.KeyPath, err)
		}

		var signer ssh.Signer
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods configured (provide --ssh-key)")
	}

	return methods, nil
}
