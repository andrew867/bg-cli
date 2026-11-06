package auth

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"github.com/andrew867/bg-cli/internal/config"
)

const keyringService = "bg-cli"

// ErrNoKeyring indicates the OS credential store is unavailable.
var ErrNoKeyring = errors.New("secure credential store unavailable")

// TokenStore persists access tokens. Implementations must never write plaintext files.
type TokenStore interface {
	Set(profile, token string) error
	Get(profile string) (string, error)
	Delete(profile string) error
}

// KeyringStore uses the OS credential store.
type KeyringStore struct {
	Service string
}

func (k KeyringStore) service() string {
	if k.Service != "" {
		return k.Service
	}
	return keyringService
}

func (k KeyringStore) account(profile string) (string, error) {
	if err := config.ValidateAlias(profile); err != nil {
		return "", err
	}
	return "profile:" + profile, nil
}

// Set stores an access token for a profile alias.
func (k KeyringStore) Set(profile, token string) error {
	acct, err := k.account(profile)
	if err != nil {
		return err
	}
	if err := keyring.Set(k.service(), acct, token); err != nil {
		return fmt.Errorf("%w: %v", ErrNoKeyring, err)
	}
	return nil
}

// Get retrieves an access token for a profile alias.
func (k KeyringStore) Get(profile string) (string, error) {
	acct, err := k.account(profile)
	if err != nil {
		return "", err
	}
	tok, err := keyring.Get(k.service(), acct)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("%w: %v", ErrNoKeyring, err)
	}
	return tok, nil
}

// Delete removes an access token for a profile alias.
func (k KeyringStore) Delete(profile string) error {
	acct, err := k.account(profile)
	if err != nil {
		return err
	}
	err = keyring.Delete(k.service(), acct)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrNoKeyring, err)
	}
	return nil
}

// MemoryStore is an in-memory TokenStore for tests. Not for production.
type MemoryStore struct {
	tokens map[string]string
	Fail   bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tokens: map[string]string{}}
}

func (m *MemoryStore) Set(profile, token string) error {
	if m.Fail {
		return ErrNoKeyring
	}
	if err := config.ValidateAlias(profile); err != nil {
		return err
	}
	if m.tokens == nil {
		m.tokens = map[string]string{}
	}
	m.tokens[profile] = token
	return nil
}

func (m *MemoryStore) Get(profile string) (string, error) {
	if m.Fail {
		return "", ErrNoKeyring
	}
	tok, ok := m.tokens[profile]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return tok, nil
}

func (m *MemoryStore) Delete(profile string) error {
	if m.Fail {
		return ErrNoKeyring
	}
	delete(m.tokens, profile)
	return nil
}
