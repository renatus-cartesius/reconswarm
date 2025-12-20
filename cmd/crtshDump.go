/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"reconswarm/internal/logging"
	"reconswarm/internal/recon/targets"

	"go.uber.org/zap"

	"github.com/spf13/cobra"
)

// crtshDumpCmd represents the crtshDump command
var crtshDumpCmd = &cobra.Command{
	Use:   "crtsh-dump [domain]",
	Short: "Dump subdomains from crt.sh for a given domain",
	Long: `Dump subdomains from crt.sh for a given domain.
This command fetches certificate data from crt.sh, extracts subdomains,
and filters them by DNS resolution to return only resolvable subdomains.

Example:
  reconswarm crtsh-dump example.com`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]

		logging.Logger().Info("Starting subdomain enumeration",
			zap.String("domain", domain))

		fmt.Printf("Fetching subdomains for domain: %s\n", domain)
		fmt.Println("This may take a few seconds...")

		subdomains, err := targets.CrtshDump(domain)
		if err != nil {
			logging.Logger().Fatal("Failed to dump subdomains",
				zap.String("domain", domain),
				zap.Error(err))
		}

		logging.Logger().Info("Subdomain enumeration completed",
			zap.String("domain", domain),
			zap.Int("count", len(subdomains)))

		if len(subdomains) == 0 {
			fmt.Printf("No resolvable subdomains found for %s\n", domain)
			return
		}

		fmt.Printf("\nFound %d resolvable subdomains for %s:\n", len(subdomains), domain)
		for _, subdomain := range subdomains {
			fmt.Printf("  - %s\n", subdomain)
		}
	},
}

func init() {
	rootCmd.AddCommand(crtshDumpCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// crtshDumpCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// crtshDumpCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
