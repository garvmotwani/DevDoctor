package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"time"
)

// serveDashboard hosts the DevDoctor dashboard on the first available
// loopback port between 8080 and 8089. Binding explicitly to 127.0.0.1 means
// project metadata never leaves the machine, even on an untrusted network.
// The callback runs only after the listener is ready, so the printed URL is
// always the address that DevDoctor actually owns.
func serveDashboard(root string, ready func(addr string)) error {
	listener, err := dashboardListener()
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           dashboardHandler(root),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ready(listener.Addr().String())
	return server.Serve(listener)
}

// dashboardListener claims a local dashboard port. A fixed port is convenient
// until it conflicts with the very development server DevDoctor is meant to
// diagnose, so try a small predictable range instead of failing unnecessarily.
func dashboardListener() (net.Listener, error) {
	var lastErr error
	for port := 8080; port <= 8089; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return listener, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("could not find a free local dashboard port (tried 8080–8089): %w", lastErr)
}

// dashboardHandler is separate from the server so it can be exercised without
// opening a port. Both routes collect fresh evidence on each request: refresh
// the page after changing a project and the results update immediately.
func dashboardHandler(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		res, err := collect(root)
		if err != nil {
			http.Error(w, "Could not scan project: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := dashboardTemplate.Execute(w, dashboardView{Result: res, Summary: summarize(res.Diagnostics)}); err != nil {
			// The response may already have started, so logging is less useful than
			// returning the template error to the server caller in a future refactor.
			return
		}
	})
	mux.HandleFunc("/api/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := collect(root)
		if err != nil {
			http.Error(w, "Could not scan project: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := renderJSON(w, res); err != nil {
			return
		}
	})
	return mux
}

type dashboardView struct {
	Result  result
	Summary jsonSummary
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>DevDoctor</title><style>
:root{color-scheme:dark;font-family:Inter,system-ui,sans-serif;background:#10131a;color:#edf2f7}body{max-width:1100px;margin:0 auto;padding:32px 20px}.hero{display:flex;justify-content:space-between;gap:20px;align-items:flex-start}h1{margin:0;font-size:2rem}h2{font-size:1.1rem;margin:0 0 14px}.sub{color:#a7b0c0;margin:8px 0}.actions a{color:#8cc8ff;text-decoration:none}.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;margin:25px 0}.card,.panel{background:#191e29;border:1px solid #303846;border-radius:12px;padding:18px}.number{font-size:2rem;font-weight:700}.warn{color:#f9c74f}.error{color:#ff7b7b}.info{color:#8cc8ff}.panel{margin:16px 0}ul{padding-left:20px;margin:0}li{margin:8px 0}.diag{border-left:4px solid #8cc8ff;padding:12px 15px;margin:12px 0;background:#141924}.diag.warning{border-color:#f9c74f}.diag.error{border-color:#ff7b7b}.meta{color:#a7b0c0;font-size:.9rem}.fix{font-family:ui-monospace,monospace;background:#0e1219;border-radius:5px;padding:7px 9px;display:inline-block}@media(max-width:650px){.hero{display:block}.grid{grid-template-columns:1fr}}
</style></head><body><main><div class="hero"><div><h1>DevDoctor <span class="meta">v` + Version + `</span></h1><p class="sub">Local-first developer diagnostics</p><p class="meta">Scanning {{.Result.Report.Root}}</p></div><div class="actions"><a href="/">Refresh</a> · <a href="/api/report">JSON API</a></div></div>
<section class="grid"><div class="card"><div class="number info">{{.Summary.Info}}</div><div class="meta">Information</div></div><div class="card"><div class="number warn">{{.Summary.Warnings}}</div><div class="meta">Warnings</div></div><div class="card"><div class="number error">{{.Summary.Errors}}</div><div class="meta">Errors</div></div></section>
<section class="panel"><h2>System</h2><span class="meta">{{.Result.System.OS}} · {{.Result.System.Architecture}}</span></section>
<section class="panel"><h2>Developer tools</h2><ul>{{range .Result.Tools}}<li><strong>{{.Tool.Name}}</strong> — {{.Status}}{{if .Version}} <span class="meta">{{.Version}}</span>{{end}}</li>{{end}}</ul></section>
<section class="panel"><h2>Project facts</h2><ul>{{range .Result.Report.Facts}}<li><strong>{{.Label}}</strong>{{if .Detail}} <span class="meta">— {{.Detail}}</span>{{end}}</li>{{else}}<li class="meta">No facts detected.</li>{{end}}</ul></section>
<section class="panel"><h2>Diagnosis</h2>{{range .Result.Diagnostics}}<article class="diag {{.Severity}}"><strong>{{.ID}} · {{.Title}}</strong><p>{{.Description}}</p><p class="meta">Evidence: {{.Evidence}}</p>{{if .SuggestedFix}}<span class="fix">{{.SuggestedFix}}</span>{{end}}</article>{{else}}<p class="meta">No diagnostics. Your project looks healthy.</p>{{end}}</section>
</main></body></html>`))

// renderDashboard is used by tests to verify that templates remain safe to
// render even when facts or diagnostics contain HTML-looking project text.
func renderDashboard(res result) (string, error) {
	var out bytes.Buffer
	err := dashboardTemplate.Execute(&out, dashboardView{Result: res, Summary: summarize(res.Diagnostics)})
	if err != nil {
		return "", fmt.Errorf("render dashboard: %w", err)
	}
	return out.String(), nil
}
