package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DIZAMU/RootCause/internal/server"
)

// TestClusterSession plays one session through the dashboard deployed in the
// cluster (spec section 18, Phase 7), the way the browser does: it reads the
// token from the served page, starts perm-01 over the API, works in the web
// terminal, and checks. From the terminal it also tries every address in
// RC_CLUSTER_PROBES ("name=ip:port,..."), all of which must be unreachable
// from a sandbox. scripts/k8s-prove.sh runs it with RC_CLUSTER_URL set; it is
// skipped otherwise.
//
// It plays on the cluster's own database, so the session counts there.
func TestClusterSession(t *testing.T) {
	base := os.Getenv("RC_CLUSTER_URL")
	if base == "" {
		t.Skip("RC_CLUSTER_URL is not set")
	}
	c := &clusterClient{t: t, base: strings.TrimSuffix(base, "/")}
	c.readToken()

	// A session left by an earlier, interrupted run ends first: give up is
	// open after the third hint.
	var active struct{ ID string }
	if c.call("GET", "/api/session", nil, &active) == http.StatusOK {
		for i := 0; i < 3; i++ {
			c.call("POST", "/api/hint", map[string]string{"session": active.ID}, nil)
		}
		if code := c.call("POST", "/api/giveup", map[string]string{"session": active.ID}, nil); code != http.StatusOK {
			t.Fatalf("give up the leftover session %s: status %d", active.ID, code)
		}
	}

	var sess struct {
		ID       string
		Status   string
		Terminal bool
	}
	if code := c.call("POST", "/api/start", map[string]string{"exercise": "perm-01-shared-folder"}, &sess); code != http.StatusOK {
		t.Fatalf("start: status %d", code)
	}
	if sess.Status != "active" || !sess.Terminal {
		t.Fatalf("started session %+v: want active with a terminal", sess)
	}
	t.Logf("session %s started", sess.ID)

	term := c.openTerminal()
	defer term.Close()
	term.send(`{"cols":120,"rows":40}`)

	// Root in the sandbox pod, in a pod named after the session.
	out := term.result(t, "who", `$(id -u)@$(hostname)`)
	if out != "0@rootcause-"+sess.ID {
		t.Errorf("terminal is not root in the session's pod: %q", out)
	}

	// Nothing outside the sandbox answers.
	for _, probe := range strings.Split(os.Getenv("RC_CLUSTER_PROBES"), ",") {
		name, addr, ok := strings.Cut(probe, "=")
		host, port, ok2 := strings.Cut(addr, ":")
		if !ok || !ok2 {
			continue
		}
		got := term.result(t, "probe-"+name, fmt.Sprintf(`$(timeout 3 bash -c 'exec 3<>/dev/tcp/%s/%s' 2>/dev/null && echo open || echo closed)`, host, port))
		if got != "closed" {
			t.Errorf("sandbox reached %s (%s): %q", name, addr, got)
		} else {
			t.Logf("sandbox cannot reach %s (%s)", name, addr)
		}
	}

	// Before the fix the check fails, the way the learner sees it.
	var res struct {
		Status string
		Check  struct{ Result string }
	}
	if code := c.call("POST", "/api/check", map[string]string{"session": sess.ID}, &res); code != http.StatusOK || res.Check.Result != "fail" {
		t.Fatalf("check before the fix: status %d, %+v", code, res)
	}

	// Fix it in the terminal, as the learner would.
	if got := term.result(t, "fix", `$(d=$(ls /srv/shares); chgrp -R "$d" "/srv/shares/$d" && chmod -R g+w "/srv/shares/$d" && chmod g+s "/srv/shares/$d" && echo done)`); got != "done" {
		t.Fatalf("fix in the terminal: %q", got)
	}
	if code := c.call("POST", "/api/check", map[string]string{"session": sess.ID}, &res); code != http.StatusOK || res.Check.Result != "pass" || res.Status != "solved" {
		t.Fatalf("check after the fix: status %d, %+v", code, res)
	}

	// The solved session takes its terminal with it.
	if reason, _ := readClose(t, term.conn, 10*time.Second); reason != "session_ended" {
		t.Errorf("terminal close reason %q, want session_ended", reason)
	}
}

type clusterClient struct {
	t     *testing.T
	base  string
	token string
}

var clusterToken = regexp.MustCompile(`<meta name="rootcause-token" content="([0-9a-f]+)"`)

func (c *clusterClient) readToken() {
	res, err := http.Get(c.base + "/")
	if err != nil {
		c.t.Fatalf("dashboard at %s: %v", c.base, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	m := clusterToken.FindSubmatch(body)
	if res.StatusCode != http.StatusOK || m == nil {
		c.t.Fatalf("dashboard page: status %d, no token", res.StatusCode)
	}
	c.token = string(m[1])
}

// call sends an API request and decodes a 200 response into out.
func (c *clusterClient) call(method, path string, body, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set(server.TokenHeader, c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 3 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusOK && out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}
	return res.StatusCode
}

type clusterTerm struct {
	conn *websocket.Conn
	t    *testing.T
}

func (c *clusterClient) openTerminal() *clusterTerm {
	d := websocket.Dialer{
		Subprotocols:     []string{server.TerminalProtocol, server.TokenProtocolPrefix + c.token},
		HandshakeTimeout: 10 * time.Second,
	}
	h := http.Header{"Origin": {c.base}}
	conn, _, err := d.Dial("ws"+strings.TrimPrefix(c.base, "http")+server.TerminalPath, h)
	if err != nil {
		c.t.Fatalf("open the terminal: %v", err)
	}
	return &clusterTerm{conn: conn, t: c.t}
}

func (tm *clusterTerm) Close() { tm.conn.Close() }

func (tm *clusterTerm) send(msg string) {
	if err := tm.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		tm.t.Fatalf("terminal write: %v", err)
	}
}

// result types a command that prints "RESULT-<name>=<value>", with value
// expanded by the shell, and returns value. The typed line itself never
// contains "RESULT-": printf joins "R" and "ESULT-" only in the output.
func (tm *clusterTerm) result(t *testing.T, name, value string) string {
	t.Helper()
	cmd := fmt.Sprintf(`printf 'R%%s\n' "ESULT-%s=%s"`, name, value)
	b, _ := json.Marshal(map[string]string{"input": cmd + "\r"})
	tm.send(string(b))
	want := regexp.MustCompile(`RESULT-` + regexp.QuoteMeta(name) + `=([^\r\n]*)\r?\n`)
	var out strings.Builder
	deadline := time.Now().Add(30 * time.Second)
	for {
		if m := want.FindStringSubmatch(out.String()); m != nil {
			return m[1]
		}
		_ = tm.conn.SetReadDeadline(deadline)
		_, data, err := tm.conn.ReadMessage()
		if err != nil {
			t.Fatalf("terminal: %v waiting for RESULT-%s; output %q", err, name, out.String())
		}
		out.Write(data)
	}
}
