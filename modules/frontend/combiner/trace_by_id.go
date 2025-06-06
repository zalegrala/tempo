package combiner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/grafana/tempo/pkg/collector"

	"github.com/gogo/protobuf/proto"
	"github.com/grafana/tempo/pkg/api"
	tempo_io "github.com/grafana/tempo/pkg/io"
	"github.com/grafana/tempo/pkg/model/trace"
	"github.com/grafana/tempo/pkg/tempopb"
)

const (
	internalErrorMsg = "internal error"
)

type TraceByIDCombiner struct {
	mu sync.Mutex

	c           *trace.Combiner
	contentType string

	code          int
	statusMessage string

	mc *collector.MetricsCollector
}

// NewTraceByID returns a trace id combiner. The trace by id combiner has a few different behaviors then the others
// - 404 is a valid response code. if all downstream jobs return 404 then it will return 404 with no body
// - translate tempopb.TraceByIDResponse to tempopb.Trace. all other combiners pass the same object through
// - runs the zipkin dedupe logic on the fully combined trace
// - encode the returned trace as either json or proto depending on the request
func NewTraceByID(maxBytes int, contentType string) Combiner {
	return &TraceByIDCombiner{
		c:           trace.NewCombiner(maxBytes, false),
		code:        http.StatusNotFound,
		contentType: contentType,
		mc:          collector.NewMetricsCollector(),
	}
}

func NewTypedTraceByID(maxBytes int, contentType string) *TraceByIDCombiner {
	return NewTraceByID(maxBytes, contentType).(*TraceByIDCombiner)
}

func (c *TraceByIDCombiner) TotalBytesProcessed() uint64 {
	return c.mc.TotalValue()
}

func (c *TraceByIDCombiner) AddResponse(r PipelineResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shouldQuit() {
		return nil
	}

	res := r.HTTPResponse()
	if res.StatusCode == http.StatusNotFound {
		// 404s are not considered errors, so we don't need to do anything.
		return nil
	}
	c.code = res.StatusCode

	if res.StatusCode != http.StatusOK {
		bytesMsg, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		c.statusMessage = string(bytesMsg)
		return nil
	}

	// Read the body
	buff, err := tempo_io.ReadAllWithEstimate(res.Body, res.ContentLength)
	if err != nil {
		c.statusMessage = internalErrorMsg
		return fmt.Errorf("error reading response body: %w", err)
	}
	_ = res.Body.Close()

	// Unmarshal the body
	resp := &tempopb.TraceByIDResponse{}
	err = resp.Unmarshal(buff)
	if err != nil {
		c.statusMessage = internalErrorMsg
		return fmt.Errorf("error unmarshalling response body: %w", err)
	}

	// Consume the trace
	_, err = c.c.Consume(resp.Trace)
	if errors.Is(err, trace.ErrTraceTooLarge) {
		c.code = http.StatusUnprocessableEntity
		c.statusMessage = fmt.Sprint(err)
		return nil
	}
	if resp.Metrics != nil {
		c.mc.Add(resp.Metrics.InspectedBytes)
	}

	return err
}

func (c *TraceByIDCombiner) HTTPFinal() (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	statusCode := c.code
	traceResult, _ := c.c.Result()

	if statusCode != http.StatusOK {
		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(strings.NewReader(c.statusMessage)),
			Header:     http.Header{},
		}, nil
	}

	// if we have no trace result just substitute and return an empty trace
	if traceResult == nil {
		traceResult = &tempopb.Trace{}
	}

	// dedupe duplicate span ids
	deduper := newDeduper()
	traceResult = deduper.dedupe(traceResult)

	// marshal in the requested format
	var buff []byte
	var err error

	if c.contentType == api.HeaderAcceptProtobuf {
		buff, err = proto.Marshal(traceResult)
	} else {
		buff, err = tempopb.MarshalToJSONV1(traceResult)
	}
	if err != nil {
		return &http.Response{}, fmt.Errorf("error marshalling response: %w content type: %s", err, c.contentType)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			api.HeaderContentType: {c.contentType},
		},
		Body:          io.NopCloser(bytes.NewReader(buff)),
		ContentLength: int64(len(buff)),
	}, nil
}

func (c *TraceByIDCombiner) StatusCode() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.code
}

// ShouldQuit returns true if the response should be returned early.
func (c *TraceByIDCombiner) ShouldQuit() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shouldQuit()
}

func (c *TraceByIDCombiner) shouldQuit() bool {
	if c.code/100 == 5 { // Bail on 5xx
		return true
	}

	// test special case for 404
	if c.code == http.StatusNotFound {
		return false
	}

	// test special case for 422
	if c.code == http.StatusUnprocessableEntity {
		return true
	}

	// bail on other 400s
	if c.code/100 == 4 {
		return true
	}

	// 2xx and 404 are OK
	return false
}
