package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"

	"github.com/logrusorgru/aurora/v3"
	"github.com/spf13/cobra"
)

var limitArg int
var noColorArg bool

var rootCmd = &cobra.Command{
	Use:     "linch [flags..]",
	Short:   "Linch is a simplistic non-recursive link validator",
	Version: "0.1",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		links, rsps, stop := startLinkHandler()
		lines := bufio.NewScanner(os.Stdin)
		go extractLinksForPaths(lines, links, stop)

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
	rootCmd.Flags().IntVarP(&limitArg, "limit", "l", 10, "limit number of concurrent connections")
	_, no_color := os.LookupEnv("NO_COLOR")
	rootCmd.Flags().BoolVarP(&noColorArg, "no-color", "n", no_color, "whether to disable colors in output")
	rootCmd.Flags().SortFlags = false
}

func (a Action) Pretty(au aurora.Aurora) string {
	switch {
	case a.Error != nil && a.Status == 0:
		return fmt.Sprintf("INTER %v: %v", au.Magenta("XXX"), a.Error)
	case a.Error != nil:
		return fmt.Sprintf("INTER %v: %v", au.Magenta(a.Status), a.Error)
	case a.Status < 300:
		return fmt.Sprintf("SUCCE %v: %v", au.Green(a.Status), a.Original)
	case a.Status == 301 || a.Status == 308:
		redir, _ := url.QueryUnescape(a.Redir)
		return fmt.Sprintf("REDIR %v: %v -> %v", au.Yellow(a.Status), a.Original, redir)
	case a.Status == 302 || a.Status == 307:
		redir, _ := url.QueryUnescape(a.Redir)
		return fmt.Sprintf("SEMIR %v: %v -> %v", au.Blue(a.Status), a.Original, redir)
	default:
		return fmt.Sprintf("ERROR %v: %v", au.Red(a.Status), a.Original)
	}
}
