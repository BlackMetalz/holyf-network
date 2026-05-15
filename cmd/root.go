package cmd

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/kernelapi"
	"github.com/BlackMetalz/holyf-network/internal/network"
	"github.com/BlackMetalz/holyf-network/internal/tui"
	"github.com/spf13/cobra"
)

// Version of the application. Overridden at build time via:
//
//	-ldflags "-X github.com/BlackMetalz/holyf-network/cmd.Version=v1.0.0"
//
// Must be var (not const) for ldflags to work.
var Version = "0.1.0-dev"

// CLI flags — stored here so rootCmd.Run() can access them
var (
	flagInterface      string
	flagRefresh        int
	flagListInterfaces bool
	flagSensitiveIP    bool
	flagHealthConfig   string
)

// rootCmd is the base command. When the user runs "holyf-network" without
// any subcommands, this is what executes.
var rootCmd = &cobra.Command{
	Use:     "holyf-network",
	Short:   "Network monitoring TUI dashboard",
	Long:    "HolyF-network - A terminal UI dashboard for monitoring network health on Linux servers.",
	Version: resolveBuildVersion(Version),

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

		healthThresholds := config.DefaultHealthThresholds()
		loadedThresholds, err := config.LoadHealthThresholds(flagHealthConfig)
		if err != nil {
			if os.IsNotExist(err) {
				loadedThresholds = healthThresholds
			} else {
				fmt.Fprintf(os.Stderr,
					"Warning: cannot load health thresholds from %q: %v (using defaults)\n",
					flagHealthConfig,
					err,
				)
			}
		} else {
			healthThresholds = loadedThresholds
		}

		// Initialize kernel API managers
		sm := kernelapi.NewSocketManager()
		cm := kernelapi.NewConntrackManager()
		fw := kernelapi.NewFirewall()
		actions.SetManagers(sm, cm, fw)
		collector.SetManagers(sm, cm)

		// Build backend indicator for status bar
		bi := kernelapi.GetBackendInfo(sm, cm, fw)
		var backendLabel string
		if bi.IsAllKernel() {
			backendLabel = "[green]API:kernel[white]"
		} else {
			backendLabel = "[yellow]API:" + bi.Summary() + "[white]"
		}

		// Launch the TUI dashboard
		app := tui.NewApp(
			ifaceName,
			flagRefresh,
			flagSensitiveIP,
			resolveBuildVersion(Version),
			healthThresholds,
		)
		app.SetBackendLabel(backendLabel)
		return app.Run()
	},
}

// Execute is called by main.go. It runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func resolveBuildVersion(defaultVersion string) string {
	version := strings.TrimSpace(defaultVersion)
	if info, ok := debug.ReadBuildInfo(); ok {
		if buildVersion := strings.TrimSpace(info.Main.Version); buildVersion != "" && buildVersion != "(devel)" {
			version = buildVersion
		}
	}
	if version == "" {
		return "dev"
	}
	return version
}

// init() runs automatically when the package is loaded.
// This is where Cobra flags are registered.
func init() {
	// Persistent flags are available to this command AND all subcommands.
	// Local flags are only for this command.
	rootCmd.Flags().StringVarP(&flagInterface, "interface", "i", "", "Network interface to monitor (default: auto-detect)")
	rootCmd.Flags().IntVarP(&flagRefresh, "refresh", "r", 30, "Refresh interval in seconds (1-300)")
	rootCmd.Flags().BoolVar(&flagListInterfaces, "list-interfaces", false, "List available network interfaces and exit")
	rootCmd.Flags().BoolVar(&flagSensitiveIP, "sensitive-ip", false, "Hide the first 2 IP octets/groups in Top Connections (for demos)")
	rootCmd.Flags().StringVar(&flagHealthConfig, "health-config", "config/health_thresholds.toml", "Health strip thresholds TOML file")

	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newReplayCmd())
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
