package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxQueryBytes     = 4096
	defaultListenWait = 5 * time.Minute
)

// CallbackResult is the one-shot OAuth redirect payload.
type CallbackResult struct {
	Code  string
	State string
	Error string
	Raw   url.Values
}

// LoopbackServer accepts exactly one OAuth callback on a loopback address.
type LoopbackServer struct {
	Listener net.Listener
	Path     string
	Timeout  time.Duration
}

// StartLoopback binds 127.0.0.1 on an ephemeral or fixed port.
func StartLoopback(redirectURI string) (*LoopbackServer, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, fmt.Errorf("parse redirect URI: %w", err)
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("private-mode redirect URI must use http loopback")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return nil, fmt.Errorf("private-mode redirect URI must target 127.0.0.1 or localhost")
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	addr := u.Host
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}
	return &LoopbackServer{
		Listener: ln,
		Path:     path,
		Timeout:  defaultListenWait,
	}, nil
}

// RedirectURI returns the exact registered callback URI including bound port.
func (s *LoopbackServer) RedirectURI(path string) string {
	if path == "" {
		path = s.Path
	}
	host := s.Listener.Addr().String()
	// Prefer 127.0.0.1 over [::] presentation.
	if strings.HasPrefix(host, "[::]") {
		host = "127.0.0.1" + strings.TrimPrefix(host, "[::]")
	}
	return "http://" + host + path
}

// Wait accepts one valid callback and shuts down.
func (s *LoopbackServer) Wait(ctx context.Context, expectedState string) (CallbackResult, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultListenWait
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan CallbackResult, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" && len(r.URL.RawQuery) > maxQueryBytes {
			http.Error(w, "query too large", http.StatusRequestURITooLong)
			select {
			case errCh <- fmt.Errorf("callback query exceeds %d bytes", maxQueryBytes):
			default:
			}
			return
		}
		if r.URL.Path != s.Path {
			http.Error(w, "unexpected path", http.StatusNotFound)
			select {
			case errCh <- fmt.Errorf("unexpected callback path %q", r.URL.Path):
			default:
			}
			return
		}
		q := r.URL.Query()
		res := CallbackResult{
			Code:  q.Get("code"),
			State: q.Get("state"),
			Error: q.Get("error"),
			Raw:   q,
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if res.Error != "" {
			_, _ = w.Write([]byte("Authorization failed. You can close this window."))
		} else {
			_, _ = w.Write([]byte("Authorization complete. You can close this window and return to bg-cli."))
		}
		select {
		case resultCh <- res:
		default:
			select {
			case errCh <- fmt.Errorf("duplicate callback rejected"):
			default:
			}
		}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(s.Listener)
	}()
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
	}()

	select {
	case <-ctx.Done():
		return CallbackResult{}, fmt.Errorf("oauth callback timed out: %w", ctx.Err())
	case err := <-errCh:
		return CallbackResult{}, err
	case res := <-resultCh:
		if res.Error != "" {
			return res, fmt.Errorf("authorization denied: %s", res.Error)
		}
		if res.Code == "" {
			return res, fmt.Errorf("callback missing authorization code")
		}
		if subtle.ConstantTimeCompare([]byte(res.State), []byte(expectedState)) != 1 {
			return res, fmt.Errorf("oauth state mismatch")
		}
		return res, nil
	}
}

// Close releases the listener.
func (s *LoopbackServer) Close() error {
	if s.Listener != nil {
		return s.Listener.Close()
	}
	return nil
}
