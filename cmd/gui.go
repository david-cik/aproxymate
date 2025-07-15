package cmd

import (
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
		port, _ := cmd.Flags().GetInt("port")
		noBrowser, _ := cmd.Flags().GetBool("no-open")
		
		log.Info("Starting GUI command", "port", port, "auto_launch", !noBrowser)
		
		gui := lib.NewGUI()
		
		// Load configurations from Viper if available
		if _, err := gui.LoadConfigFromViper(); err != nil {
			log.Warn("Failed to load configuration from viper", "error", err)
		}
		
		// Start the GUI server in a goroutine so we can handle browser opening
		serverErr := make(chan error, 1)
		serverReady := make(chan bool, 1)
		
		go func() {
			if err := gui.Start(port, serverReady); err != nil {
				serverErr <- err
			}
		}()
		
		// Wait for server to be ready, then open browser if requested
		if !noBrowser {
			go func() {
				// Wait for server to be ready
				<-serverReady
				
				url := fmt.Sprintf("http://localhost:%d", port)
				log.Info("Opening browser", "url", url)
				
				if err := openBrowser(url); err != nil {
					log.Warn("Failed to open browser automatically", "url", url, "error", err)
					fmt.Printf("🌐 Could not open browser automatically. Please visit: %s\n", url)
				} else {
					log.Info("Browser opened successfully", "url", url)
				}
			}()
		} else {
			// Even if not opening browser, we should still wait for server to be ready
			// to avoid exiting before the server starts
			go func() {
				<-serverReady
				log.Info("GUI server is ready", "port", port)
			}()
		}
		
		// Wait for server error or block indefinitely
		if err := <-serverErr; err != nil {
			log.Error("Failed to start GUI server", "port", port, "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(guiCmd)
	
	// Add flags for the gui command
	guiCmd.Flags().IntP("port", "p", 8080, "Port to run the GUI web server on")
	guiCmd.Flags().Bool("no-open", false, "Disable automatic browser opening")
}