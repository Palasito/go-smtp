// Package metrics declares and registers all Prometheus metrics for the relay.
// Import this package for its side-effects (init registers everything with the
// default registry) and reference the exported variables directly to record
// observations.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// ---------------------------------------------------------------------------
	// Connections
	// ---------------------------------------------------------------------------

	// ActiveConnections is the number of currently open SMTP sessions.
	ActiveConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "smtp_active_connections",
		Help: "Number of currently active SMTP connections.",
	})

	// SessionDuration tracks the total lifetime of SMTP sessions in seconds.
	SessionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "smtp_session_duration_seconds",
		Help:    "Lifetime of SMTP sessions from connect to disconnect.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
	})

	// ---------------------------------------------------------------------------
	// Authentication
	// ---------------------------------------------------------------------------

	// AuthTotal counts AUTH attempts, labelled by result ("success" / "failure").
	AuthTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "smtp_auth_total",
		Help: "Total SMTP authentication attempts.",
	}, []string{"result"})

	// ---------------------------------------------------------------------------
	// Messages
	// ---------------------------------------------------------------------------

	// MessagesTotal counts messages processed, labelled by final status:
	//   "sent"               — delivered to Graph API successfully
	//   "permanent_failure"  — rejected by Graph API with a non-retryable error
	//   "temporary_failure"  — Graph API transient error / retries exhausted
	//   "rejected"           — refused before Graph API (e.g. size exceeded)
	MessagesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "smtp_messages_total",
		Help: "Total messages processed, by final delivery status.",
	}, []string{"status"})

	// MessageSize is a histogram of accepted message body sizes in bytes.
	MessageSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "smtp_message_size_bytes",
		Help: "Size distribution of processed message bodies in bytes.",
		Buckets: []float64{
			1024,     // 1 KB
			10240,    // 10 KB
			102400,   // 100 KB
			1048576,  // 1 MB
			5242880,  // 5 MB
			10485760, // 10 MB
			36700160, // 35 MB (default max)
		},
	})

	// ---------------------------------------------------------------------------
	// Graph API
	// ---------------------------------------------------------------------------

	// GraphAPILatency tracks per-attempt HTTP request duration to Graph API,
	// labelled by outcome ("success" / "retryable_error" / "permanent_error" /
	// "transport_error").
	GraphAPILatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "graph_api_request_duration_seconds",
		Help:    "Latency of individual Graph API sendMail HTTP attempts.",
		Buckets: prometheus.DefBuckets,
	}, []string{"outcome"})

	// GraphAPIAttempts records how many HTTP attempts were made per message
	// (including the first attempt, so minimum value is 1).
	GraphAPIAttempts = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "graph_api_attempts_per_message",
		Help:    "Number of Graph API HTTP attempts made per message.",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	})

	// ---------------------------------------------------------------------------
	// OAuth token cache
	// ---------------------------------------------------------------------------

	// TokenCacheHits counts requests served from the in-memory token cache.
	TokenCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "oauth_token_cache_hits_total",
		Help: "Total OAuth token cache hits (token reused without contacting Azure AD).",
	})

	// TokenCacheMisses counts requests that required a live Azure AD token fetch.
	TokenCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "oauth_token_cache_misses_total",
		Help: "Total OAuth token cache misses (live Azure AD token fetch required).",
	})

	// ---------------------------------------------------------------------------
	// Failure webhook
	// ---------------------------------------------------------------------------

	// WebhookTotal counts webhook notification attempts, labelled by result
	// ("success" / "failure").
	WebhookTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_notifications_total",
		Help: "Total failure webhook notification attempts.",
	}, []string{"result"})
)

func init() {
	prometheus.MustRegister(
		ActiveConnections,
		SessionDuration,
		AuthTotal,
		MessagesTotal,
		MessageSize,
		GraphAPILatency,
		GraphAPIAttempts,
		TokenCacheHits,
		TokenCacheMisses,
		WebhookTotal,
	)
}
