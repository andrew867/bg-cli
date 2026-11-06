// Package cli wires Cobra commands for bg-cli.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

// Version is set by main / ldflags.
var Version = "0.1.1"

// Deps are injectable collaborators for tests.
type Deps struct {
	Stdout     io.Writer
	Stderr     io.Writer
	TokenStore auth.TokenStore
	Clock      output.Clock
	HTTPClient *http.Client
	BaseURL    string // tests inject local servers; production leaves empty
	OpenURL    func(string) error
	Env        func(string) string
	IsTTY      func() bool
	Now        func() time.Time
}

// GlobalFlags are shared across commands.
type GlobalFlags struct {
	Profile string
	Output  string
	Pretty  bool
	Timeout time.Duration
	Quiet   bool
	NoCache bool
}

// App holds root state.
type App struct {
	Deps  Deps
	Flags GlobalFlags
}

func (a *App) stdout() io.Writer {
	if a.Deps.Stdout != nil {
		return a.Deps.Stdout
	}
	return os.Stdout
}

func (a *App) stderr() io.Writer {
	if a.Deps.Stderr != nil {
		return a.Deps.Stderr
	}
	return os.Stderr
}

func (a *App) notice(format string, args ...any) {
	if a.Flags.Quiet {
		return
	}
	fmt.Fprintf(a.stderr(), format+"\n", args...)
}

func (a *App) writer() output.Writer {
	return output.Writer{Out: a.stdout(), Pretty: a.Flags.Pretty, Clock: a.Deps.Clock}
}

func (a *App) emit(env output.Envelope) error {
	switch a.Flags.Output {
	case "table":
		return output.WriteTable(a.stdout(), env)
	case "csv":
		return a.emitCSV(env)
	default:
		return a.writer().Write(env)
	}
}

func (a *App) emitCSV(env output.Envelope) error {
	if !env.OK {
		// Keep machine-readable errors as JSON even in csv mode.
		return a.writer().Write(env)
	}
	data, _ := env.Data.(map[string]any)
	switch env.Command {
	case "account inventory":
		inv, ok := data["inventory"].(json.RawMessage)
		if !ok {
			b, err := json.Marshal(data["inventory"])
			if err != nil {
				return err
			}
			inv = b
		}
		return output.WriteInventoryCSV(a.stdout(), inv)
	case "account currencies":
		currencies, err := currencyRawMap(data["currencies"])
		if err != nil {
			return err
		}
		return output.WriteCurrenciesCSV(a.stdout(), currencies)
	default:
		return &ExitError{Code: exitcode.Usage, Msg: "--output csv is supported for account inventory and account currencies"}
	}
}

func currencyRawMap(v any) (map[string]json.RawMessage, error) {
	switch t := v.(type) {
	case map[string]json.RawMessage:
		return t, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func (a *App) tokenStore() auth.TokenStore {
	if a.Deps.TokenStore != nil {
		return a.Deps.TokenStore
	}
	return auth.KeyringStore{}
}

// NewRoot builds the root command tree.
func NewRoot(deps Deps) *cobra.Command {
	app := &App{Deps: deps}
	root := &cobra.Command{
		Use:   "bg-cli",
		Short: "Read-only CLI for official BIG Games APIs (PS99 first)",
		Long: `bg-cli is an unofficial, owner-operated read-only client for the official BIG Games API.
Pet Simulator 99 is the first game surface; naming stays game-agnostic for future titles.

Private account commands read only the Roblox account that authorized each local profile.
Account data is a cached snapshot, not real time. This client is not endorsed by BIG Games.

Never pass client_secret as a flag. Supply it via masked prompt, stdin, or BG_CLI_CLIENT_SECRET.`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch app.Flags.Output {
			case "json", "table", "raw", "csv":
			default:
				return fmt.Errorf("invalid --output %q (want json|table|raw|csv)", app.Flags.Output)
			}
			if app.Flags.Timeout < time.Second || app.Flags.Timeout > 60*time.Second {
				return fmt.Errorf("--timeout must be between 1s and 60s")
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&app.Flags.Profile, "profile", "", "local authorization profile alias")
	root.PersistentFlags().StringVar(&app.Flags.Output, "output", "json", "output format: json|table|raw|csv")
	root.PersistentFlags().BoolVar(&app.Flags.Pretty, "pretty", false, "indent JSON output")
	root.PersistentFlags().DurationVar(&app.Flags.Timeout, "timeout", 15*time.Second, "HTTP timeout (1s-60s)")
	root.PersistentFlags().BoolVar(&app.Flags.Quiet, "quiet", false, "suppress non-error stderr notices")
	root.PersistentFlags().BoolVar(&app.Flags.NoCache, "no-cache", false, "bypass public response cache")

	root.AddCommand(newVersionCmd(app))
	root.AddCommand(newDoctorCmd(app))
	root.AddCommand(newCompletionCmd())
	root.AddCommand(newAuthCmd(app))
	root.AddCommand(newAccountCmd(app))
	root.AddCommand(newPlayerCmd(app))
	root.AddCommand(newLeaderboardCmd(app))
	root.AddCommand(newCollectionCmd(app))
	root.AddCommand(newMarketCmd(app))

	return root
}

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(app.stdout(), Version)
			return nil
		},
	}
}

// Execute runs the root command and maps errors to exit codes.
func Execute(deps Deps, args []string) int {
	root := NewRoot(deps)
	root.SetArgs(args)
	if deps.Stdout != nil {
		root.SetOut(deps.Stdout)
	}
	if deps.Stderr != nil {
		root.SetErr(deps.Stderr)
	}
	err := root.Execute()
	if err == nil {
		return exitcode.OK
	}
	if ee, ok := err.(*ExitError); ok {
		fmt.Fprintln(stderrOr(deps), ee.Error())
		return ee.Code
	}
	fmt.Fprintln(stderrOr(deps), err.Error())
	return exitcode.Usage
}

func stderrOr(deps Deps) io.Writer {
	if deps.Stderr != nil {
		return deps.Stderr
	}
	return os.Stderr
}

// ExitError carries a process exit code.
type ExitError struct {
	Code int
	Msg  string
	Err  error
}

func (e *ExitError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "error"
}

func (e *ExitError) Unwrap() error { return e.Err }

func exitf(code int, format string, args ...any) error {
	return &ExitError{Code: code, Msg: fmt.Sprintf(format, args...)}
}
