package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/cryptexus/go-tunnel/internal/tunnel"
	pb "github.com/cryptexus/go-tunnel/internal/proto"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedTunnelServiceServer
	manager *tunnel.TunnelManager
	config  *ssh.ClientConfig
}

func main() {
	socketPath := "/tmp/tunnel.sock"
	flag.Parse()

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
		"id_ed25519",  // Preferred modern key type
		"id_rsa",      // Common RSA key
		"id_ecdsa",    // ECDSA key
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

