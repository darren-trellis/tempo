package temporal

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"google.golang.org/grpc"
)

type fakeTemporalClient struct {
	client.Client
	svc workflowservice.WorkflowServiceClient
}

func (f fakeTemporalClient) WorkflowService() workflowservice.WorkflowServiceClient { return f.svc }

// pagedHistoryService serves a fixed history split into pages.
type pagedHistoryService struct {
	workflowservice.WorkflowServiceClient
	mu    sync.Mutex
	pages [][]*historypb.HistoryEvent
}

func (s *pagedHistoryService) GetWorkflowExecutionHistory(_ context.Context, req *workflowservice.GetWorkflowExecutionHistoryRequest, _ ...grpc.CallOption) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := 0
	if len(req.GetNextPageToken()) > 0 {
		i = int(req.GetNextPageToken()[0])
	}
	resp := &workflowservice.GetWorkflowExecutionHistoryResponse{History: &historypb.History{Events: s.pages[i]}}
	if i+1 < len(s.pages) {
		resp.NextPageToken = []byte{byte(i + 1)}
	}
	return resp, nil
}

func (s *pagedHistoryService) append(event *historypb.HistoryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last := len(s.pages) - 1
	s.pages[last] = append(s.pages[last], event)
}

// countingCodec strips ENC: like prefixCodec, counting what it is sent and
// how many requests overlap. It fails while broken is set.
type countingCodec struct {
	prefixCodec
	payloads, inFlight, maxInFlight atomic.Int64
	broken                          atomic.Bool
}

func (c *countingCodec) Decode(payloads []*commonpb.Payload) ([]*commonpb.Payload, error) {
	if c.broken.Load() {
		return nil, fmt.Errorf("decrypt failed")
	}
	n := c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	for {
		max := c.maxInFlight.Load()
		if n <= max || c.maxInFlight.CompareAndSwap(max, n) {
			break
		}
	}
	c.payloads.Add(int64(len(payloads)))
	time.Sleep(40 * time.Millisecond)
	return c.prefixCodec.Decode(payloads)
}

func scheduledEvent(id int64) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   id,
		EventType: enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
		Attributes: &historypb.HistoryEvent_ActivityTaskScheduledEventAttributes{
			ActivityTaskScheduledEventAttributes: &historypb.ActivityTaskScheduledEventAttributes{
				ActivityId: fmt.Sprint(id),
				Input: &commonpb.Payloads{Payloads: []*commonpb.Payload{{
					Metadata: map[string][]byte{"encoding": []byte("json/plain")},
					Data:     []byte(fmt.Sprintf(`ENC:{"n":%d}`, id)),
				}}},
			},
		},
	}
}

func pagedHistoryClient(t *testing.T, pageCount, perPage int) (*Client, *pagedHistoryService, *countingCodec) {
	t.Helper()
	svc := &pagedHistoryService{}
	id := int64(1)
	for p := 0; p < pageCount; p++ {
		var page []*historypb.HistoryEvent
		for i := 0; i < perPage; i++ {
			page = append(page, scheduledEvent(id))
			id++
		}
		svc.pages = append(svc.pages, page)
	}
	codec := &countingCodec{}
	srv := httptest.NewServer(converter.NewPayloadCodecHTTPHandler(codec))
	t.Cleanup(srv.Close)
	c := &Client{
		client:    fakeTemporalClient{svc: svc},
		config:    ConnectionConfig{CodecEndpoint: srv.URL, Namespace: "ns"},
		connected: true,
	}
	return c, svc, codec
}

func TestHistoryLoadDecodesPagesConcurrentlyInOrder(t *testing.T) {
	c, _, codec := pagedHistoryClient(t, 5, 4)
	events, err := c.GetEnhancedWorkflowHistory(context.Background(), "ns", "wf", "run")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 20 {
		t.Fatalf("events=%d", len(events))
	}
	for i, ev := range events {
		if ev.ID != int64(i+1) || ev.Input != fmt.Sprintf(`{"n":%d}`, i+1) {
			t.Fatalf("event %d out of order or not decoded: %+v", i, ev)
		}
	}
	if got := codec.maxInFlight.Load(); got < 2 || got > historyDecodeConcurrency {
		t.Fatalf("pages should decode concurrently, up to %d at once, saw %d", historyDecodeConcurrency, got)
	}
}

func TestHistoryReloadDecodesOnlyNewEvents(t *testing.T) {
	c, svc, codec := pagedHistoryClient(t, 3, 4)
	ctx := context.Background()
	if _, err := c.GetEnhancedWorkflowHistory(ctx, "ns", "wf", "run"); err != nil {
		t.Fatal(err)
	}
	if codec.payloads.Load() != 12 {
		t.Fatalf("first load should decode everything, decoded %d", codec.payloads.Load())
	}

	svc.append(scheduledEvent(13))
	events, err := c.GetEnhancedWorkflowHistory(ctx, "ns", "wf", "run")
	if err != nil {
		t.Fatal(err)
	}
	if got := codec.payloads.Load() - 12; got != 1 {
		t.Fatalf("a reload should only decode the new event, decoded %d", got)
	}
	if len(events) != 13 || events[0].Input != `{"n":1}` || events[12].Input != `{"n":13}` {
		t.Fatalf("reused and new events should both read decoded, got %d events: first %q last %q", len(events), events[0].Input, events[len(events)-1].Input)
	}

	// A different run of the same workflow shares nothing.
	if _, err := c.GetEnhancedWorkflowHistory(ctx, "ns", "wf", "other-run"); err != nil {
		t.Fatal(err)
	}
	if got := codec.payloads.Load() - 13; got != 13 {
		t.Fatalf("another run should be decoded in full, decoded %d", got)
	}
}

func TestHistoryLoadedWhileCodecFailsIsNotReused(t *testing.T) {
	c, _, codec := pagedHistoryClient(t, 2, 3)
	ctx := context.Background()
	codec.broken.Store(true)
	events, err := c.GetEnhancedWorkflowHistory(ctx, "ns", "wf", "run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(events[0].Input, "ENC:") {
		t.Fatalf("a failed decode leaves payloads encoded, got %q", events[0].Input)
	}

	codec.broken.Store(false)
	events, err = c.GetEnhancedWorkflowHistory(ctx, "ns", "wf", "run")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Input != `{"n":1}` || codec.payloads.Load() != 6 {
		t.Fatalf("encoded events must not be kept, input=%q decoded=%d", events[0].Input, codec.payloads.Load())
	}
}

func TestHistoryCacheKeepsOnlyRecentRuns(t *testing.T) {
	var h historyCache
	for i := 0; i < historyCacheSize+1; i++ {
		h.put(fmt.Sprint(i), []EnhancedHistoryEvent{{ID: 1}})
	}
	if h.get("0") != nil {
		t.Fatal("the oldest run should be dropped")
	}
	if h.get(fmt.Sprint(historyCacheSize)) == nil {
		t.Fatal("the newest run should be kept")
	}
	events := []EnhancedHistoryEvent{{ID: 1, Input: "a"}}
	h.put("mine", events)
	events[0].Input = "changed"
	if h.get("mine")[0].Input != "a" {
		t.Fatal("the cache should keep its own copy")
	}
}
