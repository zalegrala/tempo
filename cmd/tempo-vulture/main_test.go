package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/grafana/tempo/pkg/tempopb"
	v1common "github.com/grafana/tempo/pkg/tempopb/common/v1"
	v1 "github.com/grafana/tempo/pkg/tempopb/trace/v1"
	zaplogfmt "github.com/jsternberg/zap-logfmt"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestHasMissingSpans(t *testing.T) {
	cases := []struct {
		trace   *tempopb.Trace
		expeted bool
	}{
		{
			&tempopb.Trace{
				Batches: []*v1.ResourceSpans{
					{
						InstrumentationLibrarySpans: []*v1.InstrumentationLibrarySpans{
							{
								Spans: []*v1.Span{
									{
										ParentSpanId: []byte("01234"),
									},
								},
							},
						},
					},
				},
			},
			true,
		},
		{
			&tempopb.Trace{
				Batches: []*v1.ResourceSpans{
					{
						InstrumentationLibrarySpans: []*v1.InstrumentationLibrarySpans{
							{
								Spans: []*v1.Span{
									{
										SpanId: []byte("01234"),
									},
									{
										ParentSpanId: []byte("01234"),
									},
								},
							},
						},
					},
				},
			},
			false,
		},
	}

	for _, tc := range cases {
		require.Equal(t, tc.expeted, hasMissingSpans(tc.trace))
	}
}

func TestPbToAttributes(t *testing.T) {
	now := time.Now()
	cases := []struct {
		kvs        []*v1common.KeyValue
		attributes []attribute.KeyValue
	}{
		{
			kvs: []*v1common.KeyValue{
				{Key: "one", Value: &v1common.AnyValue{Value: &v1common.AnyValue_IntValue{IntValue: 2}}},
			},
			attributes: []attribute.KeyValue{
				attribute.Int("one", 2),
			},
		},
		{
			kvs: []*v1common.KeyValue{
				{Key: "time", Value: &v1common.AnyValue{Value: &v1common.AnyValue_StringValue{StringValue: now.String()}}},
				{Key: "true", Value: &v1common.AnyValue{Value: &v1common.AnyValue_BoolValue{BoolValue: true}}},
				{Key: "onepointtwo", Value: &v1common.AnyValue{Value: &v1common.AnyValue_DoubleValue{DoubleValue: 1.2}}},
			},
			attributes: []attribute.KeyValue{
				attribute.String("time", now.String()),
				attribute.Bool("true", true),
				attribute.Float64("onepointtwo", 1.2),
			},
		},
	}

	for _, tc := range cases {
		attrs := pbToAttributes(tc.kvs)

		require.Equal(t, tc.attributes, attrs)
	}
}

func TestQueryTempoAndAnalyze(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)

	config := zap.NewDevelopmentEncoderConfig()
	logger = zap.New(zapcore.NewCore(
		zaplogfmt.NewEncoder(config),
		os.Stdout,
		zapcore.DebugLevel,
	))

	cases := []struct {
		statusCode    int
		body          string
		completeTrace completeTrace
		metrics       traceMetrics
		err           error
	}{
		{
			statusCode: 404,
			err:        fmt.Errorf("trace not found"),
			completeTrace: completeTrace{
				traceID: traceID,
			},
			metrics: traceMetrics{
				requested: 1,
				notFound:  1,
			},
		},
		{
			statusCode: 200,
			body:       exampleGoodTrace,
			completeTrace: completeTrace{
				traceID:   traceID,
				spanCount: 1,
				attributes: []attribute.KeyValue{
					attribute.String("time", "2021-08-20 11:41:43.959926329 -0600 MDT m=+8234.003139815"),
				},
			},
			metrics: traceMetrics{
				requested: 1,
			},
		},
		{
			statusCode: 200,
			body:       exampleGoodTrace,
			completeTrace: completeTrace{
				traceID:   traceID,
				spanCount: 2, // wrong span count
				attributes: []attribute.KeyValue{
					attribute.String("time", "2021-08-20 11:41:43.959926329 -0600 MDT m=+8234.003139815"),
				},
			},
			metrics: traceMetrics{
				requested:          1,
				incorrectSpanCount: 1,
			},
		},
		{
			statusCode: 200,
			body:       exampleGoodTrace,
			completeTrace: completeTrace{
				traceID:   traceID,
				spanCount: 1,
				attributes: []attribute.KeyValue{
					attribute.String("incorrect", "attribute"),
				},
			},
			metrics: traceMetrics{
				requested:          1,
				incorrectAttribute: 1,
			},
		},
		{
			statusCode: 200,
			body:       exampleEmptyBatchTrace,
			completeTrace: completeTrace{
				traceID: traceID,
			},
			metrics: traceMetrics{
				requested:          1,
				notFound:           1,
				incorrectAttribute: 1,
			},
		},
		{
			statusCode: 200,
			body:       exampleChildOnlyTrace,
			completeTrace: completeTrace{
				traceID:   traceID,
				spanCount: 1,
			},
			metrics: traceMetrics{
				requested:          1,
				missingSpans:       1,
				incorrectAttribute: 1,
			},
		},
	}

	for _, tc := range cases {
		testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			res.WriteHeader(tc.statusCode)
			res.Write([]byte(tc.body))
		}))
		defer func() { testServer.Close() }()

		metrics, err := queryTempoAndAnalyze(testServer.URL, tc.completeTrace)
		require.Equal(t, tc.err, err)
		require.Equal(t, tc.metrics, metrics)
	}

}

var exampleGoodTrace string = `{ "batches": [ { "resource": { "attributes": [ { "key": "service.name", "value": { "stringValue": "unknown_service:tempo-vulture-amd64" } }, { "key": "telemetry.sdk.language", "value": { "stringValue": "go" } }, { "key": "telemetry.sdk.name", "value": { "stringValue": "opentelemetry" } }, { "key": "telemetry.sdk.version", "value": { "stringValue": "1.0.0-RC1" } } ] }, "instrumentationLibrarySpans": [ { "instrumentationLibrary": { "name": "tempo-vulture" }, "spans": [ { "traceId": "Y09aLb6miF0bZfhcyxaQbg==", "spanId": "5A8Uy4WwdXY=", "name": "write", "kind": "SPAN_KIND_INTERNAL", "startTimeUnixNano": "1629481303960011916", "endTimeUnixNano": "1629481303963039236", "attributes": [ { "key": "time", "value": { "stringValue": "2021-08-20 11:41:43.959926329 -0600 MDT m=+8234.003139815" } } ], "status": {} } ] } ] }, { "resource": { "attributes": [ { "key": "service.name", "value": { "stringValue": "unknown_service:tempo-vulture-amd64" } }, { "key": "telemetry.sdk.language", "value": { "stringValue": "go" } }, { "key": "telemetry.sdk.name", "value": { "stringValue": "opentelemetry" } }, { "key": "telemetry.sdk.version", "value": { "stringValue": "1.0.0-RC1" } } ] }, "instrumentationLibrarySpans": [ { "instrumentationLibrary": { "name": "tempo-vulture" }, "spans": [ { "traceId": "Y09aLb6miF0bZfhcyxaQbg==", "spanId": "qoYFQZ816PE=", "parentSpanId": "5A8Uy4WwdXY=", "name": "log", "kind": "SPAN_KIND_INTERNAL", "startTimeUnixNano": "1629481303960027079", "endTimeUnixNano": "1629481303960092074", "status": {} } ] } ] } ] }`

var exampleChildOnlyTrace string = `{ "batches": [ { "resource": { "attributes": [ { "key": "service.name", "value": { "stringValue": "unknown_service:tempo-vulture-amd64" } }, { "key": "telemetry.sdk.language", "value": { "stringValue": "go" } }, { "key": "telemetry.sdk.name", "value": { "stringValue": "opentelemetry" } }, { "key": "telemetry.sdk.version", "value": { "stringValue": "1.0.0-RC1" } } ] }, "instrumentationLibrarySpans": [ { "instrumentationLibrary": { "name": "tempo-vulture" }, "spans": [ { "traceId": "Y09aLb6miF0bZfhcyxaQbg==", "spanId": "qoYFQZ816PE=", "parentSpanId": "5A8Uy4WwdXY=", "name": "log", "kind": "SPAN_KIND_INTERNAL", "startTimeUnixNano": "1629481303960027079", "endTimeUnixNano": "1629481303960092074", "status": {} } ] } ] } ] }`

var exampleEmptyBatchTrace string = `{ "batches": [] }`
