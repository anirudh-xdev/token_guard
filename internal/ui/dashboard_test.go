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
	if !strings.Contains(string(ui.DashboardHTML), "<!DOCTYPE html>") {
		t.Fatal("DashboardHTML does not look like the dashboard page")
	}
}
