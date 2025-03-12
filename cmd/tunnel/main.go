package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "github.com/cryptexus/go-tunnel/internal/proto"
)

var (
	// Color functions
	successColor = color.New(color.FgGreen).SprintFunc()
	errorColor   = color.New(color.FgRed).SprintFunc()
	headerColor  = color.New(color.FgBlue, color.Bold).SprintFunc()
	infoColor    = color.New(color.FgCyan).SprintFunc()
)

var rootCmd = &cobra.Command{
	Use:   "tunnel <machine> [port_from:]port_to [[port_from:]port_to...]",
	Short: "Manage SSH tunnels",
	Long: `Create one or more SSH tunnels to a remote machine.
Examples:
  tunnel server1 8080                    # Local 8080 to remote 8080
  tunnel server1 8080:80                 # Local 8080 to remote 80
  tunnel server1 8080 9090 3000:3001    # Multiple tunnels`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		portMappings := args[1:]

		type portPair struct {
			local  int
			remote int
		}

		// Parse all port mappings first to validate
		var pairs []portPair
		for _, ports := range portMappings {
			var localPort, remotePort int
			if strings.Contains(ports, ":") {
				parts := strings.Split(ports, ":")
				var err error
				localPort, err = strconv.Atoi(parts[0])
				if err != nil {
					log.Fatalf("Invalid local port '%s': %v", parts[0], err)
				}
				remotePort, err = strconv.Atoi(parts[1])
				if err != nil {
					log.Fatalf("Invalid remote port '%s': %v", parts[1], err)
				}
			} else {
				var err error
				remotePort, err = strconv.Atoi(ports)
				if err != nil {
					log.Fatalf("Invalid port '%s': %v", ports, err)
				}
				localPort = remotePort
			}
			pairs = append(pairs, portPair{local: localPort, remote: remotePort})
		}

		conn, err := grpc.Dial("unix:///tmp/tunnel.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		client := pb.NewTunnelServiceClient(conn)
		// Create all tunnels
		for _, pair := range pairs {
			resp, err := client.CreateTunnel(context.Background(), &pb.CreateTunnelRequest{
				Host:       host,
				LocalPort:  int32(pair.local),
				RemotePort: int32(pair.remote),
			})

			if err != nil {
			fmt.Printf("%s Failed to create tunnel %d:%d: %v\n", errorColor("✗"), pair.local, pair.remote, err)
				continue
			}

			if !resp.Success {
			fmt.Printf("%s Failed to create tunnel %d:%d: %s\n", errorColor("✗"), pair.local, pair.remote, resp.Error)
				continue
			}

			fmt.Printf("%s %s:%d -> localhost:%d\n", 
				successColor("✓ Tunnel created:"),
				host,
				pair.remote,
				pair.local,
			)
		}
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active tunnels",
	Long: `List active tunnels and their status.
	
Use --watch or -w to continuously monitor tunnels in real-time.`,
	Run: func(cmd *cobra.Command, args []string) {
		watch, _ := cmd.Flags().GetBool("watch")
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

		if watch {
			// Handle Ctrl+C gracefully
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt)
			done := make(chan bool)

			go func() {
				<-sigChan
				fmt.Print("\033[?25h") // Show cursor
				fmt.Println("\nExiting watch mode...")
				done <- true
			}()

			fmt.Print("\033[?25l") // Hide cursor
			defer fmt.Print("\033[?25h") // Show cursor on exit

			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					// Clear screen and move cursor to top-left
					fmt.Print("\033[H\033[2J")
					
					resp, err = client.ListTunnels(context.Background(), &pb.ListTunnelsRequest{})
					if err != nil {
						log.Printf("Failed to list tunnels: %v", err)
						continue
					}

					if len(resp.Tunnels) == 0 {
						fmt.Printf("%s No active tunnels\n", infoColor("ℹ"))
						continue
					}

					fmt.Printf("%s %s\n", headerColor("Active Tunnels"), infoColor("(Press Ctrl+C to exit)"))
					fmt.Println()
				}
				
				displayTunnels(resp.Tunnels)
			}
		}

		if len(resp.Tunnels) == 0 {
			fmt.Printf("%s No active tunnels\n", infoColor("ℹ"))
			return
		}

		fmt.Printf("%s\n", headerColor("Active Tunnels"))
		fmt.Println()
		
		displayTunnels(resp.Tunnels)

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
			fmt.Printf("%s Failed to close tunnel: %v\n", errorColor("✗"), err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("%s Failed to close tunnel: %s\n", errorColor("✗"), resp.Error)
			os.Exit(1)
		}

		fmt.Printf("%s %s:%d\n", successColor("✓ Tunnel closed:"), host, port)
	},
}

var closeAllCmd = &cobra.Command{
	Use:   "closeall",
	Short: "Close all active tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		conn, err := grpc.Dial("unix:///tmp/tunnel.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		client := pb.NewTunnelServiceClient(conn)
		resp, err := client.CloseAllTunnels(context.Background(), &pb.CloseAllTunnelsRequest{})
		if err != nil {
			log.Fatalf("Failed to close all tunnels: %v", err)
		}

		if !resp.Success {
			log.Fatalf("Failed to close all tunnels: %s", resp.Error)
		}

		fmt.Printf("%s Closed %d tunnel(s)\n", successColor("✓"), resp.Count)
	},
}

// formatBytes converts bytes to human readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	
	hours := minutes / 60
	minutes = minutes % 60
	if hours < 24 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

// sortTunnels sorts tunnels by host and port
func sortTunnels(tunnels []*pb.ListTunnelsResponse_TunnelInfo) {
	sort.Slice(tunnels, func(i, j int) bool {
		// First sort by host
		if tunnels[i].Host != tunnels[j].Host {
			return tunnels[i].Host < tunnels[j].Host
		}
		// Then by remote port
		return tunnels[i].RemotePort < tunnels[j].RemotePort
	})
}

func displayTunnels(tunnels []*pb.ListTunnelsResponse_TunnelInfo) {
	// Sort tunnels before display
	sortTunnels(tunnels)
	for _, t := range tunnels {
		// Calculate duration since creation
		uptime := time.Since(time.Unix(t.CreatedAt, 0))
		lastActivity := time.Since(time.Unix(t.LastActivity, 0))

		// Format the basic tunnel information
		fmt.Printf("%s %s:%d -> localhost:%d\n",
			headerColor("Tunnel:"),
			t.Host,
			t.RemotePort,
			t.LocalPort,
		)

		// Format uptime and activity
		fmt.Printf("  %s %s\n", 
			infoColor("Uptime:"), 
			formatDuration(uptime),
		)
		fmt.Printf("  %s %s ago\n",
			infoColor("Last Activity:"),
			formatDuration(lastActivity),
		)

		// Format data transfer information
		fmt.Printf("  %s %s (↑) / %s (↓)\n",
			infoColor("Total Transfer:"),
			formatBytes(t.BytesSent),
			formatBytes(t.BytesReceived),
		)

		// Format current bandwidth
		fmt.Printf("  %s %.1f KB/s (↑) / %.1f KB/s (↓)\n",
			infoColor("Current Speed:"),
			t.BandwidthUp/1024,  // Convert to KB/s
			t.BandwidthDown/1024,
		)

		// Display connection information
		fmt.Printf("  %s %d active / %d total\n",
			infoColor("Connections:"),
			t.ActiveConns,
			t.TotalConns,
		)

		fmt.Println()
	}
}

func init() {
	listCmd.Flags().BoolP("watch", "w", false, "Watch mode - continuously update the display")
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(closeCmd)
	rootCmd.AddCommand(closeAllCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

