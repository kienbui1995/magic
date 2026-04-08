package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All Prometheus metrics — auto-registered on package init via promauto.
var (
	// Tasks
	MetricTasksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "magic_tasks_total",
		Help: "Total number of tasks processed.",
	}, []string{"type", "status", "worker"})

	MetricTaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "magic_task_duration_seconds",
		Help:    "Task processing duration in seconds.",
		Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"type", "worker"})

	// Workers
	MetricWorkersActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "magic_workers_active",
		Help: "Number of currently active workers.",
	}, []string{"org"})

	MetricWorkerHeartbeatLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "magic_worker_heartbeat_lag_seconds",
		Help: "Seconds since last heartbeat from each worker.",
	}, []string{"worker"})

	// Cost
	MetricCostTotalUSD = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "magic_cost_total_usd",
		Help: "Total cost in USD incurred by tasks.",
	}, []string{"org", "worker"})

	// Workflows
	MetricWorkflowStepsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "magic_workflow_steps_total",
		Help: "Total workflow steps processed.",
	}, []string{"status"})

	MetricWorkflowsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "magic_workflows_active",
		Help: "Number of currently running workflows.",
	})

	// Knowledge Hub
	MetricKnowledgeQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "magic_knowledge_queries_total",
		Help: "Total knowledge hub queries.",
	}, []string{"type"}) // type: keyword | semantic

	MetricKnowledgeEntriesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "magic_knowledge_entries_total",
		Help: "Total number of knowledge entries stored.",
	})

	// Rate limiting
	MetricRateLimitHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "magic_rate_limit_hits_total",
		Help: "Total number of rate limit rejections.",
	}, []string{"endpoint"})

	// Webhooks
	MetricWebhookDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "magic_webhook_deliveries_total",
		Help: "Total webhook delivery attempts.",
	}, []string{"status"}) // delivered | failed | dead

	MetricWebhookDeliveryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "magic_webhook_delivery_duration_seconds",
		Help:    "Webhook delivery HTTP request duration.",
		Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 10},
	})

	// Streaming
	MetricStreamsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "magic_streams_active",
		Help: "Number of currently active SSE streaming connections.",
	})

	MetricStreamDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "magic_stream_duration_seconds",
		Help:    "Duration of SSE streaming connections.",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
	})
)
