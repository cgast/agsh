package inspector

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/verify"
)

//go:embed ui/*
var embeddedUI embed.FS

// Server is the inspector HTTP + WebSocket server.
type Server struct {
	bus          events.EventBus
	store        agshctx.ContextStore
	checkpoints  verify.CheckpointManager
	registry     *platform.Registry
	mux          *http.ServeMux
	wsClients    map[*wsClient]bool
	wsMu         sync.Mutex
	startTime    time.Time

	// Approval channel for plan approval/rejection via the UI.
	approvalCh   chan ApprovalAction
}

// ApprovalAction represents an approve/reject action from the inspector UI.
type ApprovalAction struct {
	Action   string `json:"action"` // "approve" or "reject"
	Feedback string `json:"feedback,omitempty"`
}

// wsClient represents a connected WebSocket client.
type wsClient struct {
	send chan []byte
	done chan struct{}
}

// New creates a new inspector server.
func New(bus events.EventBus, store agshctx.ContextStore, registry *platform.Registry, checkpoints verify.CheckpointManager) *Server {
	s := &Server{
		bus:         bus,
		store:       store,
		checkpoints: checkpoints,
		registry:    registry,
		mux:         http.NewServeMux(),
		wsClients:   make(map[*wsClient]bool),
		startTime:   time.Now(),
		approvalCh:  make(chan ApprovalAction, 1),
	}

	// Serve embedded UI.
	uiFS, _ := fs.Sub(embeddedUI, "ui")
	s.mux.Handle("/", http.FileServer(http.FS(uiFS)))

	// WebSocket for live events.
	s.mux.HandleFunc("/ws", s.handleWebSocket)

	// REST API endpoints.
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/context", s.handleContext)
	s.mux.HandleFunc("/api/history", s.handleHistory)
	s.mux.HandleFunc("/api/checkpoints", s.handleCheckpoints)
	s.mux.HandleFunc("/api/commands", s.handleCommands)

	// Intervention endpoints.
	s.mux.HandleFunc("/api/approve", s.handleApprove)
	s.mux.HandleFunc("/api/reject", s.handleReject)

	return s
}

// Start begins serving the inspector on the given port.
func (s *Server) Start(port int) error {
	// Subscribe to all events and broadcast to WebSocket clients.
	ch := s.bus.Subscribe()
	go s.broadcastEvents(ch)

	addr := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(addr, s.mux)
}

// StartAsync starts the server in a goroutine and returns immediately.
func (s *Server) StartAsync(port int) {
	// Subscribe to all events and broadcast to WebSocket clients.
	ch := s.bus.Subscribe()
	go s.broadcastEvents(ch)

	go func() {
		addr := fmt.Sprintf(":%d", port)
		http.ListenAndServe(addr, s.mux)
	}()
}

func (s *Server) broadcastEvents(ch <-chan events.Event) {
	for ev := range ch {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}

		s.wsMu.Lock()
		for client := range s.wsClients {
			select {
			case client.send <- data:
			default:
				// Client is slow, drop the event.
			}
		}
		s.wsMu.Unlock()
	}
}

// handleWebSocket upgrades an HTTP connection to a WebSocket.
// Uses a simple polling-based approach without external deps.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Since we don't want to add gorilla/websocket as a dependency,
	// we use Server-Sent Events (SSE) instead — works in all browsers
	// and doesn't require external deps.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	client := &wsClient{
		send: make(chan []byte, 64),
		done: make(chan struct{}),
	}

	s.wsMu.Lock()
	s.wsClients[client] = true
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, client)
		s.wsMu.Unlock()
		close(client.done)
	}()

	// Send existing history as initial state.
	history := s.bus.History(time.Time{})
	for _, ev := range history {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	// Stream new events.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	history := s.bus.History(time.Time{})
	commandCount := 0
	errorCount := 0
	for _, ev := range history {
		switch ev.Type {
		case events.EventCommandEnd:
			commandCount++
		case events.EventCommandError:
			errorCount++
		}
	}

	writeJSON(w, map[string]any{
		"uptime":        time.Since(s.startTime).String(),
		"events":        len(history),
		"commands_run":  commandCount,
		"errors":        errorCount,
		"commands_total": len(s.registry.Names()),
	})
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	scopes := []string{agshctx.ScopeProject, agshctx.ScopeSession, agshctx.ScopeStep}
	result := make(map[string]map[string]any)

	for _, scope := range scopes {
		items, err := s.store.List(scope)
		if err == nil && len(items) > 0 {
			result[scope] = items
		}
	}

	writeJSON(w, result)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	history := s.bus.History(time.Time{})
	writeJSON(w, history)
}

func (s *Server) handleCheckpoints(w http.ResponseWriter, r *http.Request) {
	if s.checkpoints == nil {
		writeJSON(w, []any{})
		return
	}

	infos, err := s.checkpoints.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, infos)
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	cmds := s.registry.List("")
	infos := make([]map[string]any, len(cmds))
	for i, cmd := range cmds {
		infos[i] = map[string]any{
			"name":        cmd.Name(),
			"description": cmd.Description(),
			"namespace":   cmd.Namespace(),
		}
	}
	writeJSON(w, infos)
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	select {
	case s.approvalCh <- ApprovalAction{Action: "approve"}:
		writeJSON(w, map[string]string{"status": "approved"})
	default:
		writeJSON(w, map[string]string{"status": "no_pending_approval"})
	}
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	select {
	case s.approvalCh <- ApprovalAction{Action: "reject", Feedback: body.Feedback}:
		writeJSON(w, map[string]string{"status": "rejected"})
	default:
		writeJSON(w, map[string]string{"status": "no_pending_approval"})
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
}
