package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// decodeSocketMessage reads one socket message from r, keeping the integers of its payload
// exact.
//
// A payload is decoded into an interface{} and encoded again on its way to a handler: once to
// go through the pub/sub channel, once more into the handler's own type. Decoded the default
// way, every number becomes a float64 in between, which holds 53 bits: nanosecond timestamps
// and cursor ids (recordingTimeNano, cursorId, startTimeNano, endTimeNano) came out rounded
// to the nearest 256 ns.
func decodeSocketMessage(r io.Reader, msg *SocketMessage) error {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	if err := decoder.Decode(msg); err != nil {
		return err
	}
	msg.Payload = keepIntegersExact(msg.Payload)
	return nil
}

// unmarshalSocketMessage is decodeSocketMessage for a message held in memory: a message taken
// from the pub/sub channel, or a line of sync-pending.jsonl republished by the retry timer.
// Like json.Unmarshal it refuses anything after the message.
func unmarshalSocketMessage(data []byte, msg *SocketMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(msg); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("invalid character after socket message")
	}
	msg.Payload = keepIntegersExact(msg.Payload)
	return nil
}

// keepIntegersExact walks a value decoded with UseNumber. Numbers written as plain integers
// stay json.Number, which encodes back to the same digits. Every other number (a fraction or
// an exponent, such as a client sending a unix time as 1717243200.0) becomes a float64, as it
// always did: encoded again it turns into 1717243200, which an integer field accepts, while
// the original literal would be refused.
func keepIntegersExact(v interface{}) interface{} {
	switch value := v.(type) {
	case json.Number:
		if isIntegerLiteral(string(value)) {
			return value
		}
		if f, err := value.Float64(); err == nil {
			return f
		}
		return value
	case map[string]interface{}:
		for key, element := range value {
			value[key] = keepIntegersExact(element)
		}
		return value
	case []interface{}:
		for i, element := range value {
			value[i] = keepIntegersExact(element)
		}
		return value
	default:
		return v
	}
}

func isIntegerLiteral(s string) bool {
	if len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
