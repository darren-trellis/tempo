package command

import (
	"context"
	"strings"
	"testing"
)

func TestInjectConnectionFlagsCodecEndpoint(t *testing.T) {
	got := InjectConnectionFlags("temporal workflow show -w abc", Context{
		Address:       "us-west-2.aws.api.temporal.io:7233",
		CodecEndpoint: "https://codec.example.com",
	})
	if !strings.Contains(got, `--codec-endpoint "https://codec.example.com"`) {
		t.Fatalf("missing codec flag in %q", got)
	}
}

func TestExpandCmdCodecEndpoint(t *testing.T) {
	got, err := ExpandCmd("curl {codec_endpoint}/decode", Context{CodecEndpoint: "https://codec.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "curl https://codec.example.com/decode" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandShellCmdQuotesValues(t *testing.T) {
	got, err := ExpandShellCmd("open {match}", Context{Match: "https://example.com/a?b=1&c=2"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "open 'https://example.com/a?b=1&c=2'" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandShellCmdKeepsPayloadInert(t *testing.T) {
	payload := `it's $(echo pwned); rm -rf "$HOME" ` + "`id`"
	expanded, err := ExpandShellCmd("printf %s {value}", Context{Value: payload})
	if err != nil {
		t.Fatal(err)
	}
	out, err := Run(context.Background(), expanded)
	if err != nil {
		t.Fatalf("run %q: %v (%s)", expanded, err, out)
	}
	if out != payload {
		t.Fatalf("shell saw %q, want %q", out, payload)
	}
}

func TestExpandShellCmdCaptureGroups(t *testing.T) {
	got, err := ExpandShellCmd("open https://admin.example.com/orders/{1}", Context{Args: []string{"42"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "open https://admin.example.com/orders/'42'" {
		t.Fatalf("got %q", got)
	}
}

func TestRunPassesEnv(t *testing.T) {
	out, err := Run(context.Background(), `printf %s "$TEMPO_PATH"`, Env(Context{Path: "items.0.url"})...)
	if err != nil {
		t.Fatal(err)
	}
	if out != "items.0.url" {
		t.Fatalf("got %q", out)
	}
}

func TestOutputKeepsStderrApart(t *testing.T) {
	out, errOut, err := Output(context.Background(), `printf '{"a":1}'; echo warning >&2`)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"a":1}` || strings.TrimSpace(errOut) != "warning" {
		t.Fatalf("stdout=%q stderr=%q", out, errOut)
	}
}

func TestExpandLeavesJSONBracesAlone(t *testing.T) {
	got, err := ExpandShellCmd(`aws s3 cp s3://b/{value} - | jq '{rows: .rows, "k": 1}'`, Context{Value: "x/out.json"})
	if err != nil {
		t.Fatal(err)
	}
	if got != `aws s3 cp s3://b/'x/out.json' - | jq '{rows: .rows, "k": 1}'` {
		t.Fatalf("got %q", got)
	}
	if _, err := ExpandCmd("echo {valeu}", Context{Value: "x"}); err == nil {
		t.Fatal("a misspelled placeholder should still be reported")
	}
}
