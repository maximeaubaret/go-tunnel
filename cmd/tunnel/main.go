package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "github.com/cryptexus/go-tunnel/internal/proto"
)

var rootCmd = &cobra.Command{
	Use:   "tunnel <machine> [port_from:]port_to",
	Short: "Manage SSH tunnels",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		ports := args[1]

		var localPort, remotePort int
		if strings.Contains(ports, ":") {
			parts := strings.Split(ports, ":")
			var err error
			localPort, err = strconv.Atoi(parts[0])
			if err != nil {
				log.Fatalf("Invalid local port: %v", err)
			}
			remotePort, err = strconv.Atoi(parts[1])
			if err != nil {
				log.Fatalf("Invalid remote port: %v", err)
			}
		} else {
			var err error
			remotePort, err = strconv.Atoi(ports)
			if err != nil {
				log.Fatalf("Invalid port: %v", err)
			}
			localPort = remotePort
		}

		conn, err := grpc.Dial("unix:///tmp/tunnel.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		client := pb.NewTunnelServiceClient(conn)
		resp, err := client.CreateTunnel(context.Background(), &pb.CreateTunnelRequest{
			Host:       host,
			LocalPort:  int32(localPort),
			RemotePort: int32(remotePort),
		})

		if err != nil {
			log.Fatalf("Failed to create tunnel: %v", err)
		}

		if !resp.Success {
			log.Fatalf("Failed to create tunnel: %s", resp.Error)
		}

		fmt.Printf("Tunnel created: %s:%d -> localhost:%d\n", host, remotePort, localPort)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		conn, err := grpc.Dial("unix:///tmp/tunnel.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		client := pb.NewTunnelServiceClient(conn)
		resp, err := client.ListTunnels(context.Background(), &pb.ListTunnelsRequest{})
		if err != nil {
			log.Fatalf("Failed to list tunnels: %v", err)
		}

		if len(resp.Tunnels) == 0 {
			fmt.Println("No active tunnels")
			return
		}

		fmt.Println("Active tunnels:")
		for _, t := range resp.Tunnels {
			fmt.Printf("%s:%d -> localhost:%d\n", t.Host, t.RemotePort, t.LocalPort)
		}
	},
}

var closeCmd = &cobra.Command{
	Use:   "close <machine> <port>",
	Short: "Close a tunnel",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		port, err := strconv.Atoi(args[1])
		if err != nil {
			log.Fatalf("Invalid port: %v", err)
		}

		conn, err := grpc.Dial("unix:///tmp/tunnel.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		client := pb.NewTunnelServiceClient(conn)
		resp, err := client.CloseTunnel(context.Background(), &pb.CloseTunnelRequest{
			Host:       host,
			RemotePort: int32(port),
		})

		if err != nil {
			log.Fatalf("Failed to close tunnel: %v", err)
		}

		if !resp.Success {
			log.Fatalf("Failed to close tunnel: %s", resp.Error)
		}

		fmt.Printf("Tunnel closed: %s:%d\n", host, port)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(closeCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

