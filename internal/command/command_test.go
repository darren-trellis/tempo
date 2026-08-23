package command

import (
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
