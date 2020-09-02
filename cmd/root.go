package cmd

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var guessArg int
var timeoutArg time.Duration
var waitArg time.Duration

var rootCmd = &cobra.Command{
	Use:   "linch",
	Short: "Linch is an unixy non-recursive link validator",
	Long: `Linch is an unixy non-recursive link validator.

It helps you by checking if all links defined in your
files are still valid.  If they are not, Linch might
even offer suggestions on how to fix them.

Linch supports http, https and local file links.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		links, rsps := startLinkHandler()
		lines := bufio.NewScanner(os.Stdin)
		go extractLinksForPaths(lines, links)

		for rsp := range rsps {
			fmt.Println(rsp)
		}

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().CountVarP(&guessArg, "guess", "g", "guess level when identifying links (0-2)")
	rootCmd.Flags().DurationVarP(&timeoutArg, "timeout", "t", 3 * time.Second, "timeout for resolving http requests")
	rootCmd.Flags().DurationVarP(&waitArg, "wait", "w", 0, "wait time between http requests (default 0s)")
}
