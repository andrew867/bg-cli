package auth

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const ClientSecretEnv = "BG_CLI_CLIENT_SECRET"

// SecretSource reads the client secret without accepting CLI flags.
type SecretSource struct {
	Env    func(string) string
	Stdin  io.Reader
	Stderr io.Writer
	IsTTY  func() bool
}

func (s SecretSource) env() func(string) string {
	if s.Env != nil {
		return s.Env
	}
	return os.Getenv
}

func (s SecretSource) stderr() io.Writer {
	if s.Stderr != nil {
		return s.Stderr
	}
	return os.Stderr
}

func (s SecretSource) stdin() io.Reader {
	if s.Stdin != nil {
		return s.Stdin
	}
	return os.Stdin
}

func (s SecretSource) isTTY() bool {
	if s.IsTTY != nil {
		return s.IsTTY()
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ReadClientSecret returns the client secret from env, then masked prompt / stdin.
// Never accepts a command-line flag.
func (s SecretSource) ReadClientSecret() (string, error) {
	if v := strings.TrimSpace(s.env()(ClientSecretEnv)); v != "" {
		return v, nil
	}
	if s.isTTY() {
		fmt.Fprint(s.stderr(), "Client secret (echo disabled): ")
		fd := int(os.Stdin.Fd())
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(s.stderr())
		if err != nil {
			return "", fmt.Errorf("read client secret: %w", err)
		}
		secret := strings.TrimSpace(string(b))
		if secret == "" {
			return "", fmt.Errorf("client secret is required")
		}
		return secret, nil
	}
	// Non-interactive stdin (piped).
	reader := bufio.NewReader(s.stdin())
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read client secret from stdin: %w", err)
	}
	secret := strings.TrimSpace(line)
	if secret == "" {
		return "", fmt.Errorf("client secret is required via %s, masked prompt, or stdin", ClientSecretEnv)
	}
	return secret, nil
}
