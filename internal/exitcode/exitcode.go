// Package exitcode defines stable process exit codes for bg-cli.
package exitcode

const (
	OK              = 0
	Usage           = 2
	NotFound        = 3
	Network         = 4
	Upstream        = 5
	RateLimit       = 6
	CredentialStore = 7
	AuthRequired    = 8
	OAuthFailure    = 9
)
