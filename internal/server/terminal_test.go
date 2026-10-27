package server_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/server"
	"github.com/DIZAMU/RootCause/locales"
)

// dialTerminal opens the web terminal the way the dashboard does, with the
// given Host ("" = the listener address), Origin ("" = none) and token
// subprotocol ("" = none).
func (r *running) dialTerminal(host, origin, token string) (*websocket.Conn, *http.Response, error) {
	r.t.Helper()
	protocols := []string{server.TerminalProtocol}
	if token != "" {
		protocols = append(protocols, server.TokenProtocolPrefix+token)
	}
	d := websocket.Dialer{Subprotocols: protocols, HandshakeTimeout: 5 * time.Second}
	h := http.Header{}
	if host != "" {
		h.Set("Host", host)
	}
	if origin != "" {
		h.Set("Origin", origin)
	}
	url := "ws" + strings.TrimPrefix(r.base, "http") + server.TerminalPath
	return d.Dial(url, h)
}

func TestTerminalGuard(t *testing.T) {
	bundle, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	r := serve(t, server.Config{Bundle: bundle, Assets: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}}})
	self := "http://127.0.0.1:" + r.port

	cases := []struct{ name, host, origin, token string }{
		{"no token", "", self, ""},
		{"wrong token", "", self, strings.Repeat("0", 64)},
		{"no origin", "", "", r.token},
		{"other origin", "", "http://evil.example", r.token},
		{"origin on another port", "", "http://127.0.0.1:1", r.token},
		{"rebinding host", "evil.example:" + r.port, "http://evil.example:" + r.port, r.token},
	}
	for _, c := range cases {
		conn, res, err := r.dialTerminal(c.host, c.origin, c.token)
		if err == nil {
			conn.Close()
			t.Errorf("%s: terminal opened", c.name)
			continue
		}
		if res == nil || res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %v, want 403", c.name, err)
		}
	}
}

// readClose reads from the terminal until it closes and returns the close
// reason, and everything it printed before.
func readClose(t *testing.T, conn *websocket.Conn, within time.Duration) (reason string, out string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(within))
	var b strings.Builder
	for {
		_, data, err := conn.ReadMessage()
		if err == nil {
			b.Write(data)
			continue
		}
		var ce *websocket.CloseError
		if errors.As(err, &ce) {
			return ce.Text, b.String()
		}
		t.Fatalf("terminal did not close within %s: %v (output %q)", within, err, b.String())
	}
}

func TestTerminalFollowsSession(t *testing.T) {
	w := newWorld(t, true)
	self := "http://127.0.0.1:" + w.port

	// No active session: the socket opens and closes at once, saying why.
	conn, _, err := w.dialTerminal("", self, w.token)
	if err != nil {
		t.Fatal(err)
	}
	if reason, _ := readClose(t, conn, 5*time.Second); reason != "no_session" {
		t.Errorf("without a session: close reason %q, want no_session", reason)
	}
	conn.Close()

	var s sessionJSON
	w.api("POST", "/api/start", map[string]string{"exercise": "perm-01-shared-folder"}, 200, &s)
	var view struct{ Terminal bool }
	w.api("GET", "/api/session", nil, 200, &view)
	if !view.Terminal {
		t.Error("session view: terminal false, want true for a runtime with a terminal")
	}

	conn, res, err := w.dialTerminal("localhost:"+w.port, "http://localhost:"+w.port, w.token)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := res.Header.Get("Sec-WebSocket-Protocol"); got != server.TerminalProtocol {
		t.Errorf("subprotocol %q, want %q (the token must never be echoed)", got, server.TerminalProtocol)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"cols":100,"rows":30}`)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"input":"echo hello\r"}`)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil || string(data) != "echo hello\r" {
		t.Fatalf("terminal echo: %q, %v", data, err)
	}

	// The session ends from elsewhere (here: it expires); the terminal closes.
	sess, err := w.st.SessionByID(w.ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.eng.Expire(w.ctx, sess); err != nil {
		t.Fatal(err)
	}
	if reason, _ := readClose(t, conn, 5*time.Second); reason != "session_ended" {
		t.Errorf("after expiry: close reason %q, want session_ended", reason)
	}

	// An ended session never gets a terminal again.
	conn2, _, err := w.dialTerminal("", self, w.token)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	if reason, _ := readClose(t, conn2, 5*time.Second); reason != "no_session" {
		t.Errorf("after the session: close reason %q, want no_session", reason)
	}
}
