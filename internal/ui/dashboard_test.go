package ui_test

import (
	"strings"
	"testing"

	"tokenguard/internal/ui"
)

func TestDashboardHTMLEmbedded(t *testing.T) {
	if len(ui.DashboardHTML) == 0 {
		t.Fatal("DashboardHTML is empty; embed failed")
	}
	if !strings.Contains(string(ui.DashboardHTML), "TokenGuard") {
		t.Fatal("DashboardHTML does not look like the console page")
	}
	if !strings.Contains(string(ui.DashboardHTML), "Unlock console") {
		t.Fatal("DashboardHTML missing unlock flow")
	}
}

func TestDocsHTMLEmbedded(t *testing.T) {
	if len(ui.DocsHTML) == 0 {
		t.Fatal("DocsHTML is empty; embed failed")
	}
	if !strings.Contains(string(ui.DocsHTML), "Use TokenGuard in 5 minutes") {
		t.Fatal("DocsHTML missing quickstart title")
	}
}
