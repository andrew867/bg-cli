package security_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Canary patterns that must never appear as hard-coded production credentials.
// Synthetic fixture tokens are allowed only under testdata/fixtures.
var (
	// Concatenate so this source file does not self-match.
	robloxCookie       = regexp.MustCompile(`(?i)\.` + `ROBLOSECURITY=`)
	bearerHardcoded    = regexp.MustCompile(`Authorization:\s*Bearer\s+[A-Za-z0-9._\-]{20,}`)
	clientSecretAssign = regexp.MustCompile(`(?i)client_secret\s*=\s*"[^"]{8,}"`)
)

// Full module import paths that indicate game automation / browser driving.
var automationImportNeedles = []string{
	`"github.com/go-vgo/robotgo"`,
	`"github.com/chromedp/chromedp"`,
	`"github.com/playwright-community/playwright-go"`,
	`"github.com/tebeka/selenium"`,
	`"gobot.io/x/gobot"`,
}

func TestRepositoryRejectsSecretsAndAutomation(t *testing.T) {
	root := findModuleRoot(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".md", ".yml", ".yaml", ".json", ".txt", ".mod", ".sum":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		inFixtures := strings.Contains(rel, string(filepath.Separator)+"testdata"+string(filepath.Separator)+"fixtures")

		if robloxCookie.MatchString(text) {
			t.Errorf("%s: Roblox cookie pattern forbidden", rel)
		}
		if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
			for _, pkg := range automationImportNeedles {
				if strings.Contains(text, pkg) {
					t.Errorf("%s: automation dependency %s forbidden", rel, pkg)
				}
			}
		}
		if !inFixtures {
			if clientSecretAssign.MatchString(text) {
				t.Errorf("%s: hard-coded client_secret assignment", rel)
			}
			if bearerHardcoded.MatchString(text) && !strings.HasSuffix(rel, "_test.go") {
				t.Errorf("%s: hard-coded bearer token", rel)
			}
			lower := strings.ToLower(rel)
			if strings.Contains(lower, "client_secret") || strings.HasSuffix(rel, ".pem") {
				t.Errorf("%s: suspicious secret file name", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
