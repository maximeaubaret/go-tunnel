package tunnel

import (
	"context"
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
	Host         string
	LocalPort    int
	RemotePort   int
	client       *ssh.Client
	listener     net.Listener
	done         chan struct{}
	reconnect    chan struct{}
	sshConfig    *ssh.ClientConfig
	CreatedAt    time.Time
	LastActivity time.Time
	activityMu   sync.RWMutex
	healthCheck  *time.Ticker
	isActive     bool
	activeMu     sync.RWMutex
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

	// Configure dialer with keepalive settings
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 15 * time.Second,
	}

	// Add keepalive configuration
	conn, err := dialer.Dial("tcp", fmt.Sprintf("%s:22", host))
	if err != nil {
		return fmt.Errorf("failed to connect to host: %v", err)
	}

	// Enable TCP keepalive with more aggressive settings
	tcpConn := conn.(*net.TCPConn)
	if err := tcpConn.SetKeepAlive(true); err != nil {
		conn.Close()
		return fmt.Errorf("failed to enable keepalive: %v", err)
	}
	if err := tcpConn.SetKeepAlivePeriod(15 * time.Second); err != nil {
		conn.Close()
		return fmt.Errorf("failed to set keepalive period: %v", err)
	}
	if err := tcpConn.SetLinger(0); err != nil {
		conn.Close()
		return fmt.Errorf("failed to set linger: %v", err)
	}

	// Set more aggressive SSH keepalive settings
	sshConfig.Timeout = 30 * time.Second

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, host, sshConfig)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create SSH connection: %v", err)
	}

	client := ssh.NewClient(sshConn, chans, reqs)

	// Start SSH keepalive goroutine
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil {
					log.Printf("SSH keepalive failed: %v", err)
					return
				}
			}
		}
	}()

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", localPort))
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to start local listener: %v", err)
	}

	now := time.Now()
	tunnel := &Tunnel{
		Host:         host,
		LocalPort:    localPort,
		RemotePort:   remotePort,
		client:       client,
		listener:     listener,
		done:         make(chan struct{}),
		reconnect:    make(chan struct{}),
		sshConfig:    sshConfig,  // Store SSH config for reconnection
		CreatedAt:    now,
		LastActivity: now,
	}

	tm.tunnels[key] = tunnel
	go tunnel.start()
	return nil
}

func (t *Tunnel) start() {
	defer t.client.Close()
	defer t.listener.Close()

	// Start health check ticker
	t.healthCheck = time.NewTicker(15 * time.Second)
	defer t.healthCheck.Stop()

	// Start health check goroutine
	go t.monitorHealth()

	for {
		select {
		case <-t.done:
			return
		case <-t.reconnect:
			if err := t.reconnectSSH(); err != nil {
				log.Printf("Failed to reconnect SSH: %v", err)
				return
			}
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

func (t *Tunnel) monitorHealth() {
	for {
		select {
		case <-t.done:
			return
		case <-t.healthCheck.C:
			t.activeMu.RLock()
			isActive := t.isActive
			t.activeMu.RUnlock()

			if !isActive {
				// Test SSH connection with timeout
				done := make(chan error, 1)
				go func() {
					_, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil)
					done <- err
				}()

				select {
				case err := <-done:
					if err != nil {
						log.Printf("SSH connection test failed: %v, triggering reconnect", err)
						select {
						case t.reconnect <- struct{}{}:
						default:
						}
					}
				case <-time.After(5 * time.Second):
					log.Printf("SSH connection test timed out, triggering reconnect")
					select {
					case t.reconnect <- struct{}{}:
					default:
					}
				}
				continue
			}

			// Reset activity flag
			t.activeMu.Lock()
			t.isActive = false
			t.activeMu.Unlock()
		}
	}
}

func (t *Tunnel) updateActivity() {
	t.activityMu.Lock()
	t.LastActivity = time.Now()
	t.activityMu.Unlock()
}

func (t *Tunnel) forward(local net.Conn) {
	t.updateActivity()
	defer local.Close()
	
	// Mark tunnel as active
	t.activeMu.Lock()
	t.isActive = true
	t.activeMu.Unlock()
	
	// Set timeouts on local connection
	local.SetDeadline(time.Now().Add(30 * time.Second))
	
	// Try to establish remote connection with retry logic and timeout
	var remote net.Conn
	var err error
	connectChan := make(chan struct{})
	
	go func() {
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
						close(connectChan)
						return
					}
				}
			} else {
				log.Printf("Failed to connect to remote after 3 attempts: %v", err)
				close(connectChan)
				return
			}
		}
		close(connectChan)
	}()
	
	// Wait for connection with timeout
	select {
	case <-connectChan:
		if remote == nil {
			return
		}
	case <-time.After(10 * time.Second):
		log.Printf("Connection timeout while connecting to remote")
		return
	}
	
	defer remote.Close()
	
	// Reset deadline after successful connection
	local.SetDeadline(time.Time{})
	remote.SetDeadline(time.Time{})
	
	// Set keepalive on both connections
	if tcpConn, ok := local.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}
	if tcpConn, ok := remote.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	
	// Copy data in both directions with error handling and timeout
	copyData := func(dst net.Conn, src net.Conn, description string) {
		defer wg.Done()
		defer cancel() // Cancel context on exit
		
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				src.SetReadDeadline(time.Now().Add(5 * time.Second))
				n, err := src.Read(buf)
				if err != nil {
					if !isClosedError(err) && !isTimeout(err) {
						log.Printf("Error reading from %s: %v", description, err)
					}
					return
				}
				
				dst.SetWriteDeadline(time.Now().Add(5 * time.Second))
				_, err = dst.Write(buf[:n])
				if err != nil {
					if !isClosedError(err) && !isTimeout(err) {
						log.Printf("Error writing to %s: %v", description, err)
					}
					return
				}
				
				t.updateActivity()
			}
		}
	}

	go copyData(remote, local, "local->remote")
	go copyData(local, remote, "remote->local")

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(12 * time.Hour): // Maximum session duration
		log.Printf("Session timeout reached")
		return
	}
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
		return opErr.Err.Error() == "use of closed network connection" ||
			opErr.Err.Error() == "connection reset by peer" ||
			opErr.Err.Error() == "broken pipe"
	}
	return false
}

func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
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
		t.activityMu.RLock()
		tunnel := Tunnel{
			Host:         t.Host,
			LocalPort:    t.LocalPort,
			RemotePort:   t.RemotePort,
			CreatedAt:    t.CreatedAt,
			LastActivity: t.LastActivity,
			client:       t.client,
			listener:     t.listener,
			done:         t.done,
			reconnect:    t.reconnect,
			sshConfig:    t.sshConfig,
		}

		t.activityMu.RUnlock()
		tunnels = append(tunnels, tunnel)
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


