package tunnel

import (
	"fmt"
	"io"
	"net"
	"sync"
	"golang.org/x/crypto/ssh"
)

type TunnelManager struct {
	tunnels map[string]*Tunnel
	mu      sync.RWMutex
}

type Tunnel struct {
	Host      string
	LocalPort int
	RemotePort int
	client    *ssh.Client
	listener  net.Listener
	done      chan struct{}
}

func NewTunnelManager() *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[string]*Tunnel),
	}
}

func (tm *TunnelManager) CreateTunnel(host string, localPort, remotePort int, sshConfig *ssh.ClientConfig) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := fmt.Sprintf("%s:%d", host, remotePort)
	if _, exists := tm.tunnels[key]; exists {
		return fmt.Errorf("tunnel already exists")
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", host), sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH server: %v", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", localPort))
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to start local listener: %v", err)
	}

	tunnel := &Tunnel{
		Host:       host,
		LocalPort:  localPort,
		RemotePort: remotePort,
		client:     client,
		listener:   listener,
		done:       make(chan struct{}),
	}

	tm.tunnels[key] = tunnel
	go tunnel.start()
	return nil
}

func (t *Tunnel) start() {
	defer t.client.Close()
	defer t.listener.Close()

	for {
		select {
		case <-t.done:
			return
		default:
			local, err := t.listener.Accept()
			if err != nil {
				continue
			}

			go t.forward(local)
		}
	}
}

func (t *Tunnel) forward(local net.Conn) {
	remote, err := t.client.Dial("tcp", fmt.Sprintf("localhost:%d", t.RemotePort))
	if err != nil {
		local.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(remote, local)
		remote.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(local, remote)
		local.Close()
	}()

	wg.Wait()
}

func (tm *TunnelManager) CloseTunnel(host string, remotePort int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := fmt.Sprintf("%s:%d", host, remotePort)
	tunnel, exists := tm.tunnels[key]
	if !exists {
		return fmt.Errorf("tunnel not found")
	}

	close(tunnel.done)
	tunnel.listener.Close()
	tunnel.client.Close()
	delete(tm.tunnels, key)
	return nil
}

func (tm *TunnelManager) ListTunnels() []Tunnel {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tunnels := make([]Tunnel, 0, len(tm.tunnels))
	for _, t := range tm.tunnels {
		tunnels = append(tunnels, *t)
	}
	return tunnels
}

