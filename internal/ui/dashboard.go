package ui

import _ "embed"

// DashboardHTML is the admin dashboard, embedded so /dashboard works
// regardless of the process working directory.
//
//go:embed dashboard.html
var DashboardHTML []byte
