// The source code in file is based on file
// "src/cmd/goinstall/download.go" in the Go language distribution

package main

import (
	"http"
	"strings"
)

var dashboardURL = "http://godashboard.appspot.com/package"

// maybeReportToDashboard reports path to dashboard unless
// -dashboard=false is on command line.  It ignores errors.
func maybeReportToDashboard(path string) {
	// Only execute if command-line option "-dashboard" is true (true is the default)
	if *flag_dashboard {
		if *flag_debug {
			println("reporting \"" + path + "\" to " + dashboardURL)
		}

		// lob url to dashboard
		r, _ := http.Post(dashboardURL, "application/x-www-form-urlencoded", strings.NewReader("path="+path))
		if r != nil && r.Body != nil {
			r.Body.Close()
		}
	}
}
