package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/session"
)

// TerminalPath is the web terminal's WebSocket endpoint (v2). A browser cannot
// set headers on a WebSocket, so the client offers the token as a subprotocol,
// TokenProtocolPrefix followed by the token, next to TerminalProtocol. guard
// checks it like the token header, and checks that Origin matches Host.
const (
	TerminalPath        = "/api/terminal"
	TerminalProtocol    = "rootcause.terminal"
	TokenProtocolPrefix = "rootcause.token."
)

// terminalReadLimit bounds one message from the browser: a paste or a resize.
const terminalReadLimit = 64 << 10

// Close reasons the dashboard shows when the terminal closes.
const (
	closeSessionEnded = "session_ended"
	closeShellExited  = "shell_exited"
	closeNoSession    = "no_session"
	closeFailed       = "failed"
)

// termMsg is one message from the browser: keystrokes, or the terminal's size.
type termMsg struct {
	Input string `json:"input,omitempty"`
	Cols  uint16 `json:"cols,omitempty"`
	Rows  uint16 `json:"rows,omitempty"`
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{TerminalProtocol},
	// guard has already matched Origin against Host.
	CheckOrigin: func(*http.Request) bool { return true },
}

// protocolToken returns the token offered in Sec-WebSocket-Protocol, or "".
func protocolToken(r *http.Request) string {
	for _, p := range websocket.Subprotocols(r) {
		if tok, ok := strings.CutPrefix(p, TokenProtocolPrefix); ok {
			return tok
		}
	}
	return ""
}

// sameOrigin reports whether the request's Origin is this dashboard itself.
func sameOrigin(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Origin"), "http://"+r.Host)
}

// handleTerminal attaches a WebSocket to a shell in the active session's
// sandbox. Output goes to the browser as binary messages; the browser sends
// termMsg as text messages. The socket closes with a reason when the shell
// exits or the session ends.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess, ok, err := s.active(ctx)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade has answered the request
	}
	defer conn.Close()
	if !ok {
		closeWith(conn, closeNoSession)
		return
	}
	conn.SetReadLimit(terminalReadLimit)

	stdin, stdinW := io.Pipe()
	sizes := make(chan runtime.TermSize, 1)
	go func() {
		defer stdinW.Close()
		defer close(sizes)
		for {
			typ, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m termMsg
			if typ != websocket.TextMessage || json.Unmarshal(data, &m) != nil {
				continue
			}
			if m.Input != "" {
				if _, err := stdinW.Write([]byte(m.Input)); err != nil {
					return
				}
			}
			if m.Cols > 0 && m.Rows > 0 {
				resize(sizes, runtime.TermSize{Cols: m.Cols, Rows: m.Rows})
			}
		}
	}()

	err = s.cfg.Engine.Terminal(ctx, sess.ID, stdin, &wsWriter{conn: conn}, sizes)
	_ = stdin.Close()
	switch {
	case errors.Is(err, session.ErrSessionEnded):
		closeWith(conn, closeSessionEnded)
	case err != nil && ctx.Err() == nil:
		closeWith(conn, closeFailed)
	default:
		closeWith(conn, closeShellExited)
	}
}

// resize queues the newest terminal size, replacing one not yet taken, so a
// burst of resizes never blocks keystrokes. sizes has room for one and this is
// its only sender.
func resize(sizes chan runtime.TermSize, size runtime.TermSize) {
	select {
	case sizes <- size:
		return
	default:
	}
	select {
	case <-sizes:
	default:
	}
	sizes <- size
}

// closeWith sends a normal close frame carrying reason.
func closeWith(conn *websocket.Conn, reason string) {
	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason)
	_ = conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
}

// wsWriter sends terminal output as binary messages. One writer at a time,
// as the websocket package requires.
type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
