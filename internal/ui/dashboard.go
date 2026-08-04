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

// PortalHTML is the product sign-in / API key portal (served at /portal).
//
//go:embed portal.html
var PortalHTML []byte
