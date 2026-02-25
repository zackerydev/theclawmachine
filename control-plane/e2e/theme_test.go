//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func newThemeTestPage(t *testing.T) *rod.Page {
	t.Helper()

	bin, ok := launcher.LookPath()
	if !ok {
		t.Skip("no Chrome/Chromium browser found for Rod tests")
	}

	l := launcher.New().Bin(bin).Headless(true).NoSandbox(true)
	u, err := l.Launch()
	if err != nil {
		t.Skipf("failed to launch browser: %v", err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		l.Kill()
		t.Skipf("failed to connect Rod browser: %v", err)
	}

	t.Cleanup(func() {
		_ = browser.Close()
		l.Kill()
	})

	page := browser.MustPage(baseURL + "/").MustWaitLoad()
	return page
}

func currentTheme(page *rod.Page) string {
	return page.MustEval(`() => document.documentElement.getAttribute("data-theme")`).Str()
}

func waitForTheme(t *testing.T, page *rod.Page, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if currentTheme(page) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("theme did not become %q (got %q)", want, currentTheme(page))
}

func TestUITheme_DefaultIsDark(t *testing.T) {
	skipIfNoServer(t)
	page := newThemeTestPage(t)

	if got := currentTheme(page); got != "dark" {
		t.Fatalf("default theme = %q, want %q", got, "dark")
	}
}

func TestUITheme_SwitchToLightPersistsAfterReload(t *testing.T) {
	skipIfNoServer(t)
	page := newThemeTestPage(t)

	page.MustElement(`[data-theme-option="light"]`).MustClick()
	waitForTheme(t, page, "light")

	if stored := page.MustEval(`() => localStorage.getItem("clawmachine-theme")`).Str(); stored != "light" {
		t.Fatalf("stored theme = %q, want %q", stored, "light")
	}

	page.MustReload().MustWaitLoad()
	waitForTheme(t, page, "light")
}
