package cmd

import (
	"fmt"
	"os"

	"github.com/BlackMetalz/holyf-network/internal/network"
	"github.com/spf13/cobra"
)

// Version of the application. Updated at build time or manually.
const Version = "0.1.0"

// CLI flags — stored here so rootCmd.Run() can access them
var (
	flagInterface      string
	flagRefresh        int
	flagListInterfaces bool
)

// rootCmd is the base command. When the user runs "holyf-network" without
// any subcommands, this is what executes.
var rootCmd = &cobra.Command{
	Use:     "holyf-network",
	Short:   "Network monitoring TUI dashboard",
	Long:    "HolyF-network - A terminal UI dashboard for monitoring network health on Linux servers.",
	Version: Version,

	// RunE is like Run but returns an error. Cobra prints it for us.
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle --list-interfaces: print and exit
		if flagListInterfaces {
			return listInterfaces()
		}

		// Validate refresh interval
		if flagRefresh < 1 || flagRefresh > 300 {
			return fmt.Errorf("refresh interval must be between 1 and 300 seconds, got %d", flagRefresh)
		}

		// Resolve interface name
		ifaceName, err := resolveInterface(flagInterface)
		if err != nil {
			return err
		}

		// For now, just print what we would do (TUI comes in Epic 2)
		fmt.Printf("HolyF-network v%s\n", Version)
		fmt.Printf("Interface: %s\n", ifaceName)
		fmt.Printf("Refresh:   %ds\n", flagRefresh)
		fmt.Println("\nTUI not implemented yet — coming in Epic 2!")

		return nil
	},
}

// Execute is called by main.go. It runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// init() runs automatically when the package is loaded.
// This is where Cobra flags are registered.
func init() {
	// Persistent flags are available to this command AND all subcommands.
	// Local flags are only for this command.
	rootCmd.Flags().StringVarP(&flagInterface, "interface", "i", "", "Network interface to monitor (default: auto-detect)")
	rootCmd.Flags().IntVarP(&flagRefresh, "refresh", "r", 30, "Refresh interval in seconds (1-300)")
	rootCmd.Flags().BoolVar(&flagListInterfaces, "list-interfaces", false, "List available network interfaces and exit")
}

// listInterfaces prints all available network interfaces.
func listInterfaces() error {
	interfaces, err := network.ListInterfaces()
	if err != nil {
		return fmt.Errorf("cannot list interfaces: %w", err)
	}

	fmt.Println("Available network interfaces:")
	for _, iface := range interfaces {
		fmt.Printf("  - %s\n", iface)
	}
	return nil
}

// resolveInterface determines which interface to monitor.
// If user specified one, validate it exists. Otherwise, auto-detect.
func resolveInterface(specified string) (string, error) {
	if specified != "" {
		// User specified an interface — check it exists
		interfaces, err := network.ListInterfaces()
		if err != nil {
			// Can't list interfaces, trust the user's input
			fmt.Fprintf(os.Stderr, "Warning: cannot verify interface '%s' exists: %v\n", specified, err)
			return specified, nil
		}

		for _, iface := range interfaces {
			if iface == specified {
				return specified, nil
			}
		}
		return "", fmt.Errorf("interface '%s' not found. Available: %v", specified, interfaces)
	}

	// Auto-detect default interface
	iface, err := network.DetectDefaultInterface()
	if err != nil {
		return "", fmt.Errorf("cannot auto-detect interface: %w\nUse --interface to specify one manually", err)
	}
	return iface, nil
}
