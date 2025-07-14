package cmd

import (
	"aproxymate/lib"
	log "aproxymate/lib"
	"os"

	"github.com/spf13/cobra"
)

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

The GUI will be available at http://localhost:8080 by default.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		
		log.Info("Starting GUI command", "port", port)
		
		gui := lib.NewGUI()
		
		// Load configurations from Viper if available
		if _, err := gui.LoadConfigFromViper(); err != nil {
			log.Warn("Failed to load configuration from viper", "error", err)
		}
		
		if err := gui.Start(port); err != nil {
			log.Error("Failed to start GUI server", "port", port, "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(guiCmd)
	
	// Add flags for the gui command
	guiCmd.Flags().IntP("port", "p", 8080, "Port to run the GUI web server on")
}