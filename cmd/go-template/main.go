package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	_ "go.uber.org/automaxprocs"
)

const (
	sensorUpdateInterval = 10 * time.Second
	readTimeout          = 5 * time.Second
	writeTimeout         = 10 * time.Second
	idleTimeout          = 120 * time.Second
	exampleSensorValue   = 41.0
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "an error occurred: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger, err := newJSONLogger(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return err
	}

	defer func() {
		err = logger.Sync()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error syncing logger: %s\n", err)
		}
	}()

	sensorValue := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sensor_value",
			Help: "Current value of the sensor",
		},
		[]string{"sensor"},
	)
	prometheus.MustRegister(sensorValue)

	http.Handle("/metrics", promhttp.Handler())

	go updateSensorValues(sensorValue, logger)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      nil, // Default handler
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal("Failed to start server")
	}

	logger.Info("Starting server on :8080")
	return nil
}

func updateSensorValues(sensorValue *prometheus.GaugeVec, logger *zap.Logger) {
	for {
		value := getSensorValue("sensor1")
		sensorValue.WithLabelValues("sensor1").Set(value)
		logger.Info("Sensor value updated", zap.String("sensor", "sensor1"), zap.Float64("value", value))

		// Add logic for other sensors as needed

		time.Sleep(sensorUpdateInterval)
	}
}

func newJSONLogger(level string) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	if level != "" {
		var zapLevel zap.AtomicLevel
		err := zapLevel.UnmarshalText([]byte(level))
		if err != nil {
			return nil, err
		}
		config.Level = zapLevel
	}
	return config.Build()
}

func getSensorValue(_ string) float64 {
	// Implement the logic to fetch the sensor values here
	// Example: returning a dummy value
	return exampleSensorValue
}
