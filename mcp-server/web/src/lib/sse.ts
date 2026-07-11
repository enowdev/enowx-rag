import { useEffect, useRef, useState, useCallback } from 'react'

export interface SSEEvent {
  type: string
  timestamp: string
  data?: Record<string, unknown>
}

export interface UseEventsResult {
  events: SSEEvent[]
  connected: boolean
  clear: () => void
}

/**
 * useEvents: SSE hook that connects to /api/events via EventSource.
 * Returns a list of events and connection status.
 */
export function useEvents(maxEvents: number = 50): UseEventsResult {
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [connected, setConnected] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    const es = new EventSource('/api/events')
    esRef.current = es

    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)

    // Listen for all event types by handling named events.
    // EventSource dispatches named events for `event:` fields.
    // We also handle the generic 'message' event as a fallback.
    const handler = (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        setEvents((prev) => {
          const next = [
            { type: e.type === 'message' ? data.type || 'message' : e.type, timestamp: data.timestamp || new Date().toISOString(), data: data.data || data },
            ...prev,
          ]
          return next.slice(0, maxEvents)
        })
      } catch {
        // ignore parse errors
      }
    }

    // EventSource doesn't have a wildcard listener, so we listen to common event types.
    const eventTypes = [
      'index_started',
      'index_completed',
      'index_failed',
      'query_executed',
      'project_created',
      'project_deleted',
      'points_deleted',
      'documents_indexed',
      'message',
    ]
    eventTypes.forEach((t) => es.addEventListener(t, handler))

    return () => {
      es.close()
      setConnected(false)
    }
  }, [maxEvents])

  const clear = useCallback(() => setEvents([]), [])

  return { events, connected, clear }
}
