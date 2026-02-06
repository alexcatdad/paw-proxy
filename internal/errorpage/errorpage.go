// internal/errorpage/errorpage.go
package errorpage

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

// NotFound renders an HTML page when no route is registered for the host.
// SECURITY: All dynamic content is HTML-escaped to prevent XSS.
func NotFound(w http.ResponseWriter, host string, appName string, activeRoutes []string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)

	var routeList string
	if len(activeRoutes) > 0 {
		var items []string
		for _, r := range activeRoutes {
			items = append(items, fmt.Sprintf(
				"<li><a href=\"https://%s.test\">%s.test</a></li>",
				html.EscapeString(r), html.EscapeString(r),
			))
		}
		routeList = "<h2>Active Routes</h2><ul>" + strings.Join(items, "") + "</ul>"
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>Not Found - %s</title>
<style>
body { font-family: -apple-system, system-ui, sans-serif; max-width: 600px; margin: 80px auto; padding: 0 20px; color: #333; }
h1 { color: #e74c3c; }
pre { background: #f4f4f4; padding: 12px; border-radius: 6px; overflow-x: auto; }
a { color: #3498db; }
ul { list-style: none; padding: 0; }
li { padding: 4px 0; }
</style>
</head><body>
<h1>No app at %s</h1>
<p>Start your dev server with:</p>
<pre>up -n %s &lt;your-dev-command&gt;</pre>
%s
</body></html>`,
		html.EscapeString(host),
		html.EscapeString(host),
		html.EscapeString(appName),
		routeList,
	)
}

// UpstreamDown renders an HTML page when the upstream server is not responding.
// Includes auto-refresh so the page reloads when the dev server starts.
// SECURITY: All dynamic content is HTML-escaped to prevent XSS.
func UpstreamDown(w http.ResponseWriter, host string, upstream string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)

	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="2">
<title>Waiting - %s</title>
<style>
body { font-family: -apple-system, system-ui, sans-serif; max-width: 600px; margin: 80px auto; padding: 0 20px; color: #333; }
h1 { color: #e67e22; }
.spinner { display: inline-block; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
</head><body>
<h1><span class="spinner">&#x21bb;</span> %s is not responding</h1>
<p>The dev server at <code>%s</code> isn't running.</p>
<p>Waiting for it to start... <small>(auto-refreshing every 2s)</small></p>
</body></html>`,
		html.EscapeString(host),
		html.EscapeString(host),
		html.EscapeString(upstream),
	)
}
