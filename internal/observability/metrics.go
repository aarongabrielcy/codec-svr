package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	TCPConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "codec_tcp_connections_total",
		Help: "Total de conexiones TCP aceptadas",
	})
	HandshakeOK = promauto.NewCounter(prometheus.CounterOpts{
		Name: "codec_handshake_ok_total",
		Help: "Total de handshakes IMEI ok",
	})
	PacketsRecv = promauto.NewCounter(prometheus.CounterOpts{
		Name: "codec_packets_received_total",
		Help: "Total de paquetes AVL recibidos (frames)",
	})
	RecordsAck = promauto.NewCounter(prometheus.CounterOpts{
		Name: "codec_records_ack_total",
		Help: "Total de registros AVL confirmados (ACK a Teltonika)",
	})
	ParseErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "codec_parse_errors_total",
		Help: "Errores al parsear Codec8E",
	})
	RedisSetErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "codec_redis_set_errors_total",
		Help: "Errores al escribir estados en Redis",
	})
	IOChanges = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codec_io_changes_total",
		Help: "Cambios detectados de IO por clave",
	}, []string{"key"})
	ParseLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "codec_parse_latency_seconds",
		Help:    "Latencia del parseo por frame",
		Buckets: prometheus.DefBuckets,
	})
)

func ObserveParseLatency(start time.Time) {
	ParseLatency.Observe(time.Since(start).Seconds())
}

func StartMetricsServer(port string) {
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	_ = http.ListenAndServe(":"+port, nil)
}
