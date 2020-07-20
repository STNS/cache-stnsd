package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version string

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of cache-stnsd",
	Long:  `All software has versions. This is cache-stnsd's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("cache-stnsd v%s\n", version)
	},
}
