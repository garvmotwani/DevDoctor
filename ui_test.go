package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardListenerUsesLoopback(t *testing.T) {
	listener, err := dashboardListener()
	if err != nil {
		t.Fatalf("dashboardListener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.IsLoopback() || addr.Port < 8080 || addr.Port > 8089 {
		t.Errorf("listener address = %v, want loopback port 8080–8089", listener.Addr())
	}
}

func TestDashboardRendersAndEscapesProjectText(t *testing.T) {
	res := sampleResult()
	res.Report.Facts[0].Label = `<script>alert("xss")</script>`
	html, err := renderDashboard(res)
	if err != nil {
		t.Fatalf("renderDashboard: %v", err)
	}
	if strings.Contains(html, `<script>alert`) {
		t.Fatalf("dashboard did not escape project text: %s", html)
	}
	for _, want := range []string{"DevDoctor", "Developer tools", "Diagnosis", "&lt;script&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestDashboardAPI(t *testing.T) {
	stubTools(t)
	dir := writeFixture(t, map[string]string{"package.json": "{}"})
	h := dashboardHandler(dir)

	t.Run("json report", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/report", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
			t.Errorf("content type = %q", rr.Header().Get("Content-Type"))
		}
		if !strings.Contains(rr.Body.String(), `"diagnostics"`) {
			t.Errorf("API response did not contain a report: %s", rr.Body.String())
		}
	})

	t.Run("reject non get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/report", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
		}
	})
}
