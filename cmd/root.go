package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/logrusorgru/aurora/v3"
	"github.com/spf13/cobra"
)

var limitArg int
var timeoutArg time.Duration
var waitArg time.Duration
var noColorArg bool

var rootCmd = &cobra.Command{
	Use:     "linch [flags..]",
	Short:   "Linch is a simplistic non-recursive link validator",
	Version: "0.1",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		links, rsps := startLinkHandler()
		lines := bufio.NewScanner(os.Stdin)
		go extractLinksForPaths(lines, links)

		au := aurora.NewAurora(!noColorArg)
		for rsp := range rsps {
			fmt.Println(rsp.Pretty(au))
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().IntVarP(&limitArg, "limit", "l", 100, "limit number of concurrent connections")
	rootCmd.Flags().DurationVarP(&timeoutArg, "timeout", "t", 3*time.Second, "timeout for resolving requests")
	rootCmd.Flags().DurationVarP(&waitArg, "wait", "w", 0, "wait time between individual requests (default 0s)")
	_, no_color := os.LookupEnv("NO_COLOR")
	rootCmd.Flags().BoolVarP(&noColorArg, "no-color", "n", no_color, "whether to disable colors in output")
	rootCmd.Flags().SortFlags = false
}

func (a Action) Pretty(au aurora.Aurora) string {
	switch {
	case a.Err != nil && a.Status == 0:
		return fmt.Sprintf("INTER %v: %v", au.Magenta("XXX"), a.Err)
	case a.Err != nil:
		return fmt.Sprintf("INTER %v: %v", au.Magenta(a.Status), a.Err)
	case a.Status < 300:
		return fmt.Sprintf("SUCCE %v: %v", au.Green(a.Status), a.Original.String())
	case a.Status == 301 || a.Status == 308:
		redir, _ := url.QueryUnescape(a.Redir.String())
		return fmt.Sprintf("REDIR %v: %v -> %v", au.Yellow(a.Status), a.Original.String(), redir)
	case a.Status == 302 || a.Status == 307:
		redir, _ := url.QueryUnescape(a.Redir.String())
		return fmt.Sprintf("SEMIR %v: %v -> %v", au.Blue(a.Status), a.Original.String(), redir)
	default:
		return fmt.Sprintf("ERROR %v: %v", au.Red(a.Status), a.Original.String())
	}
}
