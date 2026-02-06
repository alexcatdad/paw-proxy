package errorpage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotFoundRendersHTML(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w, "myapp.test", "myapp", []string{"dashboard", "api"})

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "myapp.test") {
		t.Error("expected host in body")
	}
	if !strings.Contains(body, "up -n myapp") {
		t.Error("expected up command in body")
	}
	if !strings.Contains(body, "dashboard.test") {
		t.Error("expected active route in body")
	}
}

func TestNotFoundNoRoutes(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w, "myapp.test", "myapp", nil)

	body := w.Body.String()
	if strings.Contains(body, "Active Routes") {
		t.Error("should not show routes section when none active")
	}
}

func TestUpstreamDownRendersHTML(t *testing.T) {
	w := httptest.NewRecorder()
	UpstreamDown(w, "myapp.test", "localhost:3000")

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "auto-refreshing") {
		t.Error("expected auto-refresh mention")
	}
	if !strings.Contains(body, "localhost:3000") {
		t.Error("expected upstream in body")
	}
	if !strings.Contains(body, `meta http-equiv="refresh"`) {
		t.Error("expected meta refresh tag")
	}
}

func TestNotFoundEscapesHTML(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w, "<script>alert(1)</script>.test", "xss", []string{"<img onerror=alert(1)>"})

	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("XSS: unescaped script tag in body")
	}
	if strings.Contains(body, "<img onerror") {
		t.Error("XSS: unescaped img tag in route list")
	}
}

func TestUpstreamDownEscapesHTML(t *testing.T) {
	w := httptest.NewRecorder()
	UpstreamDown(w, "<script>alert(1)</script>.test", "<img onerror=alert(1)>")

	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("XSS: unescaped script tag in host")
	}
	if strings.Contains(body, "<img onerror") {
		t.Error("XSS: unescaped img tag in upstream")
	}
}
