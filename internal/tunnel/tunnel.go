package tunnel

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
	"golang.org/x/crypto/ssh"
)

type TunnelManager struct {
	tunnels map[string]*Tunnel
	mu      sync.RWMutex
}

type Tunnel struct {
	Host       string
	LocalPort  int
	RemotePort int
	client     *ssh.Client
	listener   net.Listener
	done       chan struct{}
	reconnect  chan struct{}
	sshConfig  *ssh.ClientConfig
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
		reconnect:  make(chan struct{}),
		sshConfig:  sshConfig,  // Store SSH config for reconnection
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
				if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
					log.Printf("Temporary accept error: %v, retrying...", err)
					continue
				}
				log.Printf("Fatal accept error: %v, stopping tunnel", err)
				return
			}

			go t.forward(local)
		}
	}
}

func (t *Tunnel) forward(local net.Conn) {
	defer local.Close()
	
	localAddr := local.RemoteAddr().String()
	log.Printf("New connection from %s to %s:%d", localAddr, t.Host, t.RemotePort)
	
	// Try to establish remote connection with retry logic
	var remote net.Conn
	var err error
	for attempts := 0; attempts < 3; attempts++ {
		remote, err = t.client.Dial("tcp", fmt.Sprintf("localhost:%d", t.RemotePort))
		if err == nil {
			break
		}
		
		if attempts < 2 {
			log.Printf("Failed to connect to remote (attempt %d/3): %v, retrying...", attempts+1, err)
			time.Sleep(time.Second * time.Duration(attempts+1))
			
			// Try to reconnect SSH if needed
			if t.client.Conn.Wait() != nil { // SSH connection is dead
				if err := t.reconnectSSH(); err != nil {
					log.Printf("Failed to reconnect SSH: %v", err)
					return
				}
			}
		} else {
			log.Printf("Failed to connect to remote after 3 attempts: %v", err)
			return
		}
	}
	
	if remote == nil {
		return
	}
	defer remote.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	
	// Copy data in both directions with error handling
	copyData := func(dst net.Conn, src net.Conn, description string) {
		defer wg.Done()
		_, err := io.Copy(dst, src)
		if err != nil && !isClosedError(err) {
			log.Printf("Error in %s stream: %v", description, err)
		}
	}

	go copyData(remote, local, "local->remote")
	go copyData(local, remote, "remote->local")

	wg.Wait()
}

func (t *Tunnel) reconnectSSH() error {
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", t.Host), t.sshConfig)
	if err != nil {
		return fmt.Errorf("failed to reconnect SSH: %v", err)
	}
	
	oldClient := t.client
	t.client = client
	oldClient.Close()
	
	return nil
}

// isClosedError checks if the error is due to using closed network connection
func isClosedError(err error) bool {
	if err == io.EOF {
		return true
	}
	if opErr, ok := err.(*net.OpError); ok {
		return opErr.Err.Error() == "use of closed network connection"
	}
	return false
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

func (tm *TunnelManager) CloseAllTunnels() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	count := len(tm.tunnels)
	for key, tunnel := range tm.tunnels {
		close(tunnel.done)
		tunnel.listener.Close()
		tunnel.client.Close()
		delete(tm.tunnels, key)
	}
	return count
}


