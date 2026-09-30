package mcp

import (
	"strings"
	"testing"
)

func TestResolveSpotifyAndTerminal(t *testing.T) {
	sp, err := ResolveApp("spotify")
	if err != nil {
		t.Fatal(err)
	}
	if sp.Kind != KindSnap && !strings.Contains(strings.ToLower(sp.ID), "spotify") {
		t.Fatalf("spotify: %+v", sp)
	}

	term, err := ResolveApp("Terminal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(term.ID), "terminal") {
		t.Fatalf("terminal: %+v", term)
	}
}

func TestResolveWhatsAppIsWebOrPWA(t *testing.T) {
	app, err := ResolveApp("whatsapp")
	if err != nil {
		t.Fatal(err)
	}
	if app.Kind != KindWeb && app.Kind != KindPWA {
		t.Fatalf("expected web or pwa, got %+v", app)
	}
}

func TestResolveNotesnookFlatpak(t *testing.T) {
	app, err := ResolveApp("Notesnook")
	if err != nil {
		t.Skip(err)
	}
	if app.Kind != KindFlatpak {
		t.Fatalf("got %+v", app)
	}
}

func TestCloseWebRefusesBrowserKill(t *testing.T) {
	app := &AppEntry{Name: "whatsapp", Kind: KindWeb, URL: "https://web.whatsapp.com"}
	out, err := StopApp(app)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "status=fail") {
		t.Fatal(out)
	}
}
