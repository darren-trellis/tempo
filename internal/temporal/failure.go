package temporal

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	failurepb "go.temporal.io/api/failure/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func populateFailureDetails(event *EnhancedHistoryEvent, failure *failurepb.Failure) {
	if failure == nil {
		return
	}
	event.Failure = failure.GetMessage()
	event.FailureSource = failure.GetSource()
	event.FailureStackTrace = failure.GetStackTrace()
	event.FailureCause = formatFailureCause(failure.GetCause())
}

// decodeEncodedFailures unpacks failures whose SDK moved the message and stack
// trace into encoded attributes, leaving "Encoded failure" as the message.
func decodeEncodedFailures(failure *failurepb.Failure) {
	for f := failure; f != nil; f = f.GetCause() {
		encoded := f.GetEncodedAttributes()
		if encoded == nil || len(encoded.GetData()) == 0 {
			continue
		}
		var attrs struct {
			Message    string `json:"message"`
			StackTrace string `json:"stack_trace"`
		}
		if err := json.Unmarshal(encoded.GetData(), &attrs); err != nil {
			continue
		}
		if attrs.Message != "" {
			f.Message = attrs.Message
		}
		if attrs.StackTrace != "" {
			f.StackTrace = attrs.StackTrace
		}
		f.EncodedAttributes = nil
	}
}

// eventAttributesJSON renders event attributes the way Temporal UI does, with
// payloads shown as their decoded values rather than base64.
func eventAttributesJSON(msg proto.Message) string {
	raw, err := protojson.Marshal(msg)
	if err != nil {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	out, err := json.Marshal(decodePayloadValues(value))
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func decodePayloadValues(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if decoded, ok := payloadJSONValue(v); ok {
			return decoded
		}
		for key, item := range v {
			v[key] = decodePayloadValues(item)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = decodePayloadValues(item)
		}
		return v
	default:
		return value
	}
}

// payloadJSONValue reads a payload that protojson rendered as base64 metadata
// and data back into the value it carries.
func payloadJSONValue(m map[string]any) (any, bool) {
	metadata, ok := m["metadata"].(map[string]any)
	if !ok || len(m) > 2 {
		return nil, false
	}
	encodingB64, ok := metadata["encoding"].(string)
	if !ok {
		return nil, false
	}
	encoding, err := base64.StdEncoding.DecodeString(encodingB64)
	if err != nil {
		return nil, false
	}
	dataB64, _ := m["data"].(string)
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, false
	}
	switch string(encoding) {
	case "binary/null":
		return nil, true
	case "json/plain":
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, false
		}
		return decoded, true
	}
	return nil, false
}

func formatFailureCause(failure *failurepb.Failure) string {
	if failure == nil {
		return ""
	}

	var parts []string
	for f := failure; f != nil; f = f.GetCause() {
		var line strings.Builder
		if f.GetSource() != "" {
			line.WriteString(f.GetSource())
			line.WriteString(": ")
		}
		line.WriteString(f.GetMessage())
		if f.GetStackTrace() != "" {
			line.WriteString("\n")
			line.WriteString(f.GetStackTrace())
		}
		parts = append(parts, line.String())
	}

	return strings.Join(parts, "\n\nCaused by: ")
}
