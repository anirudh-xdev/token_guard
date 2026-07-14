package ui

import _ "embed"

// DashboardHTML is the authenticated developer console (served at /dashboard).
//
//go:embed dashboard.html
var DashboardHTML []byte

// DocsHTML is the public developer guide (served at /docs). No secrets required.
//
//go:embed docs.html
var DocsHTML []byte
