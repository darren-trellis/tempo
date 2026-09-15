package temporal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/debug"
	"testing"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/sdk/converter"
)

type prefixCodec struct{}

func (prefixCodec) Encode(payloads []*commonpb.Payload) ([]*commonpb.Payload, error) {
	out := make([]*commonpb.Payload, len(payloads))
	for i, p := range payloads {
		out[i] = &commonpb.Payload{
			Metadata: map[string][]byte{"encoding": []byte("json/plain")},
			Data:     append([]byte("ENC:"), p.GetData()...),
		}
	}
	return out, nil
}

func (prefixCodec) Decode(payloads []*commonpb.Payload) ([]*commonpb.Payload, error) {
	out := make([]*commonpb.Payload, len(payloads))
	for i, p := range payloads {
		data := p.GetData()
		if bytes.HasPrefix(data, []byte("ENC:")) {
			data = data[4:]
		}
		out[i] = &commonpb.Payload{
			Metadata: map[string][]byte{"encoding": []byte("json/plain")},
			Data:     data,
		}
	}
	return out, nil
}

func TestDecodePayloadsInMessages(t *testing.T) {
	srv := httptest.NewServer(converter.NewPayloadCodecHTTPHandler(prefixCodec{}))
	t.Cleanup(srv.Close)

	codec := newRemotePayloadCodec(srv.URL, "test-ns")

	event := &historypb.HistoryEvent{
		Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
				Input: &commonpb.Payloads{
					Payloads: []*commonpb.Payload{{
						Metadata: map[string][]byte{"encoding": []byte("json/plain")},
						Data:     []byte(`ENC:{"orderId":"123"}`),
					}},
				},
			},
		},
	}

	decodePayloadsInMessages(codec, event)

	got := event.GetWorkflowExecutionStartedEventAttributes().GetInput().GetPayloads()[0].GetData()
	if string(got) != `{"orderId":"123"}` {
		t.Fatalf("decoded payload = %q, want JSON", got)
	}
	if formatted := formatPayloads(event.GetWorkflowExecutionStartedEventAttributes().GetInput()); formatted != `{"orderId":"123"}` {
		t.Fatalf("formatPayloads = %q", formatted)
	}
}

func TestDecodePayloadsSkipsSearchAttributes(t *testing.T) {
	srv := httptest.NewServer(converter.NewPayloadCodecHTTPHandler(prefixCodec{}))
	t.Cleanup(srv.Close)

	raw := []byte(`["category=WorkflowTaskFailed"]`)
	info := &workflowpb.WorkflowExecutionInfo{
		SearchAttributes: &commonpb.SearchAttributes{
			IndexedFields: map[string]*commonpb.Payload{
				temporalReportedProblemsAttr: {
					Metadata: map[string][]byte{"encoding": []byte("json/plain"), "type": []byte("KeywordList")},
					Data:     raw,
				},
			},
		},
		Memo: &commonpb.Memo{
			Fields: map[string]*commonpb.Payload{
				"note": {Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: []byte("ENC:ok")},
			},
		},
	}

	decodePayloadsInMessages(newRemotePayloadCodec(srv.URL, "test-ns"), info)

	got := info.GetSearchAttributes().GetIndexedFields()[temporalReportedProblemsAttr].GetData()
	if string(got) != string(raw) {
		t.Fatalf("search attribute mutated by codec: %q", got)
	}
	if string(info.GetMemo().GetFields()["note"].GetData()) != "ok" {
		t.Fatalf("memo should still decode: %q", info.GetMemo().GetFields()["note"].GetData())
	}
}

func TestRemotePayloadCodecSendsNamespace(t *testing.T) {
	var gotNamespace, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNamespace = r.Header.Get("X-Namespace")
		gotPath = r.URL.Path
		converter.NewPayloadCodecHTTPHandler(prefixCodec{}).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	codec := newRemotePayloadCodec(srv.URL+"/{namespace}", "dsadr-dev.cvhrv")
	_, err := codec.Decode([]*commonpb.Payload{{
		Data: []byte("ENC:ok"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotNamespace != "dsadr-dev.cvhrv" {
		t.Fatalf("X-Namespace = %q", gotNamespace)
	}
	if gotPath != "/dsadr-dev.cvhrv/decode" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestProbeCodec(t *testing.T) {
	srv := httptest.NewServer(converter.NewPayloadCodecHTTPHandler(prefixCodec{}))
	t.Cleanup(srv.Close)
	if err := ProbeCodec(srv.URL, "test-ns"); err != nil {
		t.Fatal(err)
	}
	if err := ProbeCodec("http://127.0.0.1:1", "test-ns"); err == nil {
		t.Fatal("expected error for unreachable codec")
	}
}

func TestNormalizeCodecEndpoint(t *testing.T) {
	got := normalizeCodecEndpoint("https://devtools.example.ts.net:4001\nexport TEMPORAL_CODEC_URL_STG=https://devtools.example.ts.net:4002/decode")
	if got != "https://devtools.example.ts.net:4001" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeCodecEndpoint("https://codec.example.com/decode/"); got != "https://codec.example.com" {
		t.Fatalf("stripped suffix = %q", got)
	}
}

func TestLiveCodec(t *testing.T) {
	if os.Getenv("TEMPORAL_TS_API_KEY") == "" {
		t.Skip("no Temporal credentials")
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v\n%s", r, debug.Stack())
		}
	}()

	cfg := ConnectionConfig{
		Address:       envOr("TEMPORAL_HOST", "us-west-2.aws.api.temporal.io:7233"),
		Namespace:     envOr("TEMPORAL_TS_NAMESPACE", "dsadr-dev.cvhrv"),
		APIKey:        os.Getenv("TEMPORAL_TS_API_KEY"),
		CodecEndpoint: envOr("TEMPORAL_CODEC_URL_DEV", os.Getenv("TEMPORAL_CODEC_URL")),
	}
	t.Logf("codec=%q", cfg.CodecEndpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client, err := NewClient(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	wfs, _, err := client.ListWorkflows(ctx, cfg.Namespace, ListOptions{PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("listed %d workflows", len(wfs))
	if len(wfs) == 0 {
		return
	}

	events, err := client.GetEnhancedWorkflowHistory(ctx, cfg.Namespace, wfs[0].ID, wfs[0].RunID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("events=%d", len(events))
	for _, ev := range events {
		if ev.Input != "" || ev.Result != "" {
			t.Logf("event %d %s input=%d result=%d details=%d", ev.ID, ev.Type, len(ev.Input), len(ev.Result), len(ev.Details))
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func TestDecodePayloadsInMessagesNoopWithoutCodec(t *testing.T) {
	payload := &commonpb.Payload{Data: []byte("ENC:secret")}
	event := &historypb.HistoryEvent{
		Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
				Input: &commonpb.Payloads{Payloads: []*commonpb.Payload{payload}},
			},
		},
	}
	decodePayloadsInMessages(nil, event)
	if string(payload.GetData()) != "ENC:secret" {
		t.Fatalf("payload mutated without codec: %q", payload.GetData())
	}
}
