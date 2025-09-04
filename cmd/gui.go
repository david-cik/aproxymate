package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"aproxymate/lib"
	log "aproxymate/lib/logger"
)

// openBrowser opens the URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

// guiCmd represents the gui command
var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Start the web-based GUI for managing proxy connections",
	Long: `Start a web-based graphical user interface for managing Kubernetes proxy connections.
	
The GUI provides an easy-to-use interface where you can:
- Add multiple proxy configurations
- Specify Kubernetes cluster, remote host, local port, and remote port
- Start and stop proxy connections with a single click
- Monitor connection status
- Load pre-configured proxy settings from a config file

Configuration File:
You can pre-populate proxy configurations by creating a config file. Use:
  aproxymate config init
to generate a sample configuration file, then start the GUI with:
  aproxymate gui --config path/to/your/config.yaml

The GUI will be available at http://localhost:8080 by default and will automatically open in your browser.
Use --no-open flag to disable automatic browser opening.`,
	Run: func(cmd *cobra.Command, args []string) {
		opCtx, _ := log.StartOperation(context.Background(), "gui", "start")
		defer func() {
			if r := recover(); r != nil {
				opCtx.Complete("gui_start", fmt.Errorf("panic: %v", r))
				panic(r)
			}
		}()

		port, _ := cmd.Flags().GetInt("port")
		noBrowser, _ := cmd.Flags().GetBool("no-open")

		opCtx.Debug("Starting GUI command", "port", port, "auto_launch", !noBrowser)
		log.LogUserAction("start_gui", "gui_server", map[string]any{
			"port":         port,
			"auto_browser": !noBrowser,
		})

		gui := lib.NewGUI()

		// Load configurations from Viper if available
		timer := log.StartTimer("config_load")
		numConfigs, err := gui.LoadConfigFromViper()
		timer.Stop()

		if err != nil {
			// Check if this is a missing cluster error
			if numConfigs > 0 {
				outputCtx := lib.NewSimpleOutputContext()
				outputCtx.UserError("❌ Failed to load configuration: %v\n", err)
				fmt.Println("\nYour configuration has proxy entries but some are missing Kubernetes cluster specifications.")
				fmt.Println("Please fix this by running:")
				fmt.Println("  aproxymate config fix")
				fmt.Println("\nThen start the GUI again:")
				fmt.Printf("  aproxymate gui --port %d\n", port)
				opCtx.Complete("gui_start", err)
				os.Exit(1)
			} else {
				opCtx.Warn("Failed to load configuration from viper", "error", err.Error())
			}
		} else {
			opCtx.Info("Configuration loaded successfully", "num_configs", numConfigs)
		}

		// Start the GUI server in a goroutine so we can handle browser opening
		serverErr := make(chan error, 1)
		serverReady := make(chan bool, 1)

		go func() {
			log.LogGUIStart(port)
			if err := gui.Start(port, serverReady); err != nil {
				log.LogGUIStop(port, err)
				serverErr <- err
			}
		}()

		// Wait for server to be ready, then open browser if requested
		if !noBrowser {
			go func() {
				// Wait for server to be ready
				<-serverReady

				url := fmt.Sprintf("http://localhost:%d", port)

				opCtx.Debug("Attempting to open browser", "url", url)
				if err := openBrowser(url); err != nil {
					outputCtx := lib.NewOutputContext(opCtx)
					outputCtx.Warn("Failed to open browser automatically", "🌐 Could not open browser automatically. Please visit: %s\n", url)
				} else {
					opCtx.Debug("Browser opened successfully", "url", url)
					log.LogUserAction("open_browser", "browser", map[string]any{
						"url":         url,
						"auto_opened": true,
					})
				}
			}()
		} else {
			// Even if not opening browser, we should still wait for server to be ready
			// to avoid exiting before the server starts
			go func() {
				<-serverReady
				opCtx.Debug("GUI server is ready", "port", port)
			}()
		}

		// Wait for server error or block indefinitely
		if err := <-serverErr; err != nil {
			opCtx.Error("Failed to start GUI server", err, "port", port)
			opCtx.Complete("gui_start", err)
			os.Exit(1)
		}

		opCtx.Complete("gui_start", nil)
	},
}

func init() {
	rootCmd.AddCommand(guiCmd)

	// Add flags for the gui command
	guiCmd.Flags().IntP("port", "p", 8080, "Port to run the GUI web server on")
	guiCmd.Flags().Bool("no-open", false, "Disable automatic browser opening")
}
