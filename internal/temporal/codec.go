package temporal

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/converter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const codecRequestTimeout = 10 * time.Second

func normalizeCodecEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if i := strings.IndexAny(endpoint, "\r\n"); i >= 0 {
		endpoint = strings.TrimSpace(endpoint[:i])
	}
	endpoint = strings.TrimRight(endpoint, "/")
	for _, suffix := range []string{"/decode", "/encode"} {
		if strings.HasSuffix(endpoint, suffix) {
			endpoint = strings.TrimSuffix(endpoint, suffix)
			endpoint = strings.TrimRight(endpoint, "/")
		}
	}
	return endpoint
}

func ProbeCodec(endpoint, namespace string) error {
	endpoint = normalizeCodecEndpoint(endpoint)
	if endpoint == "" {
		return nil
	}
	endpoint = strings.ReplaceAll(endpoint, "{namespace}", namespace)
	codec := converter.NewRemotePayloadCodec(converter.RemotePayloadCodecOptions{
		Endpoint: endpoint,
		Client:   http.Client{Timeout: 3 * time.Second},
		ModifyRequest: func(req *http.Request) error {
			if namespace != "" {
				req.Header.Set("X-Namespace", namespace)
			}
			return nil
		},
	})
	_, err := codec.Decode([]*commonpb.Payload{{
		Metadata: map[string][]byte{"encoding": []byte("json/plain")},
		Data:     []byte("null"),
	}})
	return err
}

func newRemotePayloadCodec(endpoint, namespace string) converter.PayloadCodec {
	endpoint = normalizeCodecEndpoint(endpoint)
	endpoint = strings.ReplaceAll(endpoint, "{namespace}", namespace)
	return converter.NewRemotePayloadCodec(converter.RemotePayloadCodecOptions{
		Endpoint: endpoint,
		Client:   http.Client{Timeout: codecRequestTimeout},
		ModifyRequest: func(req *http.Request) error {
			if namespace != "" {
				req.Header.Set("X-Namespace", namespace)
			}
			return nil
		},
	})
}

func dataConverterFor(connConfig ConnectionConfig) converter.DataConverter {
	if connConfig.CodecEndpoint == "" {
		return nil
	}
	return converter.NewCodecDataConverter(
		converter.GetDefaultDataConverter(),
		newRemotePayloadCodec(connConfig.CodecEndpoint, connConfig.Namespace),
	)
}

func (c *Client) payloadCodec(namespace string) converter.PayloadCodec {
	c.mu.RLock()
	endpoint := c.config.CodecEndpoint
	defaultNS := c.config.Namespace
	c.mu.RUnlock()
	if endpoint == "" {
		return nil
	}
	if namespace == "" {
		namespace = defaultNS
	}
	return newRemotePayloadCodec(endpoint, namespace)
}

func asProtoMessages[T proto.Message](items []T) []proto.Message {
	msgs := make([]proto.Message, len(items))
	for i, item := range items {
		msgs[i] = item
	}
	return msgs
}

func decodePayloadsInMessages(codec converter.PayloadCodec, msgs ...proto.Message) {
	if codec == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			if sdkLogger != nil {
				sdkLogger.Error("codec decode panicked", "error", r, "stack", string(debug.Stack()))
			}
		}
	}()
	var payloads []*commonpb.Payload
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		collectPayloads(msg.ProtoReflect(), &payloads)
	}
	if len(payloads) == 0 {
		return
	}
	decoded, err := codec.Decode(payloads)
	if err != nil || len(decoded) != len(payloads) {
		if sdkLogger != nil {
			sdkLogger.Warn("codec decode failed", "error", err)
		}
		return
	}
	for i, p := range payloads {
		if p == nil || decoded[i] == nil {
			continue
		}
		p.Metadata = decoded[i].GetMetadata()
		p.Data = decoded[i].GetData()
	}
}

func collectPayloads(m protoreflect.Message, out *[]*commonpb.Payload) {
	collectPayloadsDepth(m, out, 0)
}

func collectPayloadsDepth(m protoreflect.Message, out *[]*commonpb.Payload, depth int) {
	if !m.IsValid() || depth > 32 {
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Kind() != protoreflect.MessageKind {
			return true
		}
		if fd.Name() == "search_attributes" {
			return true
		}
		switch {
		case fd.IsMap():
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					collectFromMessage(mv.Message(), out, depth+1)
					return true
				})
			}
		case fd.IsList():
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				collectFromMessage(list.Get(i).Message(), out, depth+1)
			}
		default:
			collectFromMessage(v.Message(), out, depth+1)
		}
		return true
	})
}

func collectFromMessage(m protoreflect.Message, out *[]*commonpb.Payload, depth int) {
	if !m.IsValid() {
		return
	}
	switch p := m.Interface().(type) {
	case *commonpb.SearchAttributes:
		return
	case *commonpb.Payloads:
		for _, payload := range p.GetPayloads() {
			if payload != nil {
				*out = append(*out, payload)
			}
		}
	case *commonpb.Payload:
		if p != nil {
			*out = append(*out, p)
		}
	default:
		collectPayloadsDepth(m, out, depth+1)
	}
}
