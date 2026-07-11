package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SSE handles GET /api/events.
// Returns a Server-Sent Events stream that publishes events from the
// core.Service EventBus. The connection stays open until the client
// disconnects.
func (h *Handlers) SSE(w http.ResponseWriter, r *http.Request) {
	// Verify the ResponseWriter supports flushing (required for SSE).
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to events from the service EventBus.
	ch := h.svc.Events().Subscribe()
	defer h.svc.Events().Unsubscribe(ch)

	// Send an initial comment to establish the connection immediately.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Set up a heartbeat ticker to keep the connection alive.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		case <-ticker.C:
			// Send a heartbeat comment to keep the connection alive.
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
