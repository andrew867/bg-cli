package cli

import (
	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate completion scripts for common shells.

Examples:

  # Bash (Linux)
  source <(bg-cli completion bash)

  # Zsh
  source <(bg-cli completion zsh)

  # Fish
  bg-cli completion fish | source

  # PowerShell (Windows)
  bg-cli completion powershell | Out-String | Invoke-Expression

To load completions for every new session, install the script into your shell's
completion directory (see your shell docs). Completions never print secrets.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			default:
				return exitf(2, "unsupported shell %q (want bash|zsh|fish|powershell)", args[0])
			}
		},
	}
	return cmd
}
