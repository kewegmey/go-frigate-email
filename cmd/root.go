package cmd

import (
	"os"

	"github.com/kewegmey/go-frigate-email/frigate_email"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "go-frigate-email",
	Short: "go-frigate-email",
	Long:  `Connects to mqtt and sends messages to a specified email address.`,
	Run: func(cmd *cobra.Command, args []string) {
		configValue, _ := cmd.Flags().GetString("config")
		frigate_email.Start(configValue)

	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.go-ipsc.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().StringP("config", "c", "config.yaml", "Configuration file path.")
}
