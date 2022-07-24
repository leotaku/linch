package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	limitArg   int
	noColorArg bool
	sedModeArg bool
)

var rootCmd = &cobra.Command{
	Use:     "linch [flags..]",
	Short:   "Linch is a simplistic non-recursive link validator",
	Version: "0.1",
	Example: `  $ echo README.md | linch
  $ find ../notes | linch
  $ fd | linch --sed-mode | parallel -j1`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		links, rsps := startLinkHandler()
		lines := bufio.NewScanner(os.Stdin)
		go extractLinksForPaths(lines, links)

		color.NoColor = noColorArg
		for rsp := range rsps {
			line := ""
			switch {
			case sedModeArg:
				line = rsp.SedCommand()
			default:
				line = rsp.Pretty()
			}
			if line != "" {
				fmt.Println(line)
			}
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
	_, noColor := os.LookupEnv("NO_COLOR")
	rootCmd.Flags().BoolVarP(&noColorArg, "no-color", "n", noColor, "whether to disable colors in output")
	rootCmd.Flags().BoolVarP(&sedModeArg, "sed-mode", "s", false, "whether to emit sed commands")
	rootCmd.Flags().SortFlags = false
}

func (a Action) Pretty() string {
	switch {
	case a.Error != nil && a.Status == 0:
		return fmt.Sprintf("INTER %v: %v %v", color.New(color.FgMagenta).Sprint("XXX"), a.Error, a.Original.URL)
	case a.Error != nil:
		return fmt.Sprintf("INTER %v: %v", color.New(color.FgMagenta).Sprint(a.Status), a.Error)
	case a.Status < 300:
		return fmt.Sprintf("SUCCE %v: %v", color.New(color.FgGreen).Sprint(a.Status), a.Original.URL)
	case a.Status == 301 || a.Status == 308:
		redir, _ := url.QueryUnescape(a.Redir)
		return fmt.Sprintf("REDIR %v: %v -> %v", color.New(color.FgYellow).Sprint(a.Status), a.Original.URL, redir)
	case a.Status == 302 || a.Status == 307:
		redir, _ := url.QueryUnescape(a.Redir)
		return fmt.Sprintf("SEMIR %v: %v -> %v", color.New(color.FgBlue).Sprint(a.Status), a.Original.URL, redir)
	default:
		return fmt.Sprintf("ERROR %v: %v", color.New(color.FgRed).Sprint(a.Status), a.Original.URL)
	}
}

func (a Action) SedCommand() string {
	switch {
	case a.Status == 301 || a.Status == 308:
		return fmt.Sprintf("sed -i -e 's^%v^%v^g' %v", a.Original.URL, a.Redir, a.Original.Path)
	case a.Error != nil:
		return fmt.Sprintf("echo error '%v'", a.Error)
	default:
		return ""
	}
}
