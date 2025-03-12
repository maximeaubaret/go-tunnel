package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/maximeaubaret/go-tunnel/internal/version"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	pb "github.com/maximeaubaret/go-tunnel/internal/proto"
	"github.com/maximeaubaret/go-tunnel/internal/tunnel"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedTunnelServiceServer
	manager *tunnel.TunnelManager
	config  *ssh.ClientConfig
}

func (s *server) CreateTunnel(ctx context.Context, req *pb.CreateTunnelRequest) (*pb.CreateTunnelResponse, error) {
	log.Printf("Creating tunnel: %s:%d -> localhost:%d", req.Host, req.RemotePort, req.LocalPort)
	err := s.manager.CreateTunnel(req.Host, int(req.LocalPort), int(req.RemotePort), s.config)
	if err != nil {
		return &pb.CreateTunnelResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &pb.CreateTunnelResponse{
		Success: true,
	}, nil
}

func (s *server) CloseTunnel(ctx context.Context, req *pb.CloseTunnelRequest) (*pb.CloseTunnelResponse, error) {
	log.Printf("Closing tunnel: %s:%d", req.Host, req.RemotePort)
	err := s.manager.CloseTunnel(req.Host, int(req.RemotePort))
	if err != nil {
		return &pb.CloseTunnelResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &pb.CloseTunnelResponse{
		Success: true,
	}, nil
}

func (s *server) CloseAllTunnels(ctx context.Context, req *pb.CloseAllTunnelsRequest) (*pb.CloseAllTunnelsResponse, error) {
	log.Printf("Closing all tunnels...")
	count := s.manager.CloseAllTunnels()
	log.Printf("Closed %d tunnel(s)", count)
	return &pb.CloseAllTunnelsResponse{
		Success: true,
		Count:   int32(count),
		Error:   "",
	}, nil
}

func (s *server) ListTunnels(ctx context.Context, req *pb.ListTunnelsRequest) (*pb.ListTunnelsResponse, error) {
	tunnels := s.manager.ListTunnels()
	var pbTunnels []*pb.ListTunnelsResponse_TunnelInfo

	for _, t := range tunnels {
		pbTunnels = append(pbTunnels, &pb.ListTunnelsResponse_TunnelInfo{
			Host:          t.Host,
			LocalPort:     int32(t.LocalPort),
			RemotePort:    int32(t.RemotePort),
			LastActivity:  t.LastActivity.Unix(),
			CreatedAt:     t.CreatedAt.Unix(),
			BytesSent:     t.BytesSent,
			BytesReceived: t.BytesReceived,
			BandwidthUp:   t.BandwidthUp,
			BandwidthDown: t.BandwidthDown,
			ActiveConns:   t.ActiveConns,
			TotalConns:    t.TotalConns,
		})
	}

	return &pb.ListTunnelsResponse{
		Tunnels: pbTunnels,
	}, nil
}

func main() {
	socketPath := "/tmp/tunnel.sock"
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("tunneld version %s (%s) built on %s\n", 
			version.Version, version.Commit, version.Date)
		return
	}

	// Cleanup any existing socket file
	if err := os.RemoveAll(socketPath); err != nil {
		log.Printf("Warning: could not remove existing socket: %v", err)
	}

	// Load SSH config (you might want to make this configurable)
	config := &ssh.ClientConfig{
		User: os.Getenv("USER"),
		Auth: []ssh.AuthMethod{
			getSSHAuth(),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterTunnelServiceServer(s, &server{
		manager: tunnel.NewTunnelManager(),
		config:  config,
	})

	// Handle shutdown gracefully
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		s.GracefulStop()
		// Cleanup socket file on shutdown
		if err := os.RemoveAll(socketPath); err != nil {
			log.Printf("Warning: could not remove socket file on shutdown: %v", err)
		}
	}()

	log.Printf("Server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func getSSHAuth() ssh.AuthMethod {
	// Check for custom SSH key path first
	if keyPath := os.Getenv("SSH_KEY_PATH"); keyPath != "" {
		if auth := tryLoadKey(keyPath); auth != nil {
			return auth
		}
		log.Printf("Warning: couldn't use specified SSH_KEY_PATH: %s", keyPath)
	}

	// Common key file names to try
	keyFiles := []string{
		"id_ed25519", // Preferred modern key type
		"id_rsa",     // Common RSA key
		"id_ecdsa",   // ECDSA key
	}

	sshDir := os.ExpandEnv("$HOME/.ssh")
	for _, keyFile := range keyFiles {
		keyPath := filepath.Join(sshDir, keyFile)
		if auth := tryLoadKey(keyPath); auth != nil {
			return auth
		}
	}

	log.Printf("Warning: no valid SSH keys found in %s", sshDir)
	return nil
}

func tryLoadKey(keyPath string) ssh.AuthMethod {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		// Skip logging for non-existent files
		if !os.IsNotExist(err) {
			log.Printf("Warning: couldn't read SSH key %s: %v", keyPath, err)
		}
		return nil
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		// Try parsing with passphrase if available
		if passphrase := os.Getenv("SSH_KEY_PASSPHRASE"); passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
			if err != nil {
				log.Printf("Warning: couldn't parse SSH key %s with passphrase: %v", keyPath, err)
				return nil
			}
		} else {
			log.Printf("Warning: couldn't parse SSH key %s (set SSH_KEY_PASSPHRASE if key is encrypted): %v", keyPath, err)
			return nil
		}
	}

	log.Printf("Successfully loaded SSH key: %s", keyPath)
	return ssh.PublicKeys(signer)
}
