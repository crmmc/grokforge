package xai

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// ---------- gRPC-Web frame helpers ----------

// grpcwebEncode wraps data in a gRPC-Web data frame: [0x00][4-byte big-endian length][data].
func grpcwebEncode(data []byte) []byte {
	buf := make([]byte, 5+len(data))
	buf[0] = 0x00
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(data)))
	copy(buf[5:], data)
	return buf
}

// grpcwebParseTrailers parses a gRPC-Web binary response body and extracts
// the grpc-status code and grpc-message from the trailer frame (flag 0x80).
// Returns an error if no trailer frame is found or grpc-status is missing.
func grpcwebParseTrailers(body []byte) (code int, msg string, err error) {
	i, n := 0, len(body)
	foundTrailer := false
	for i < n {
		if n-i < 5 {
			break
		}
		flag := body[i]
		length := int(binary.BigEndian.Uint32(body[i+1 : i+5]))
		i += 5
		if n-i < length {
			break
		}
		payload := body[i : i+length]
		i += length

		if flag&0x80 != 0 {
			foundTrailer = true
			trailers := parseTrailerPayload(payload)
			raw, ok := trailers["grpc-status"]
			if !ok {
				return -1, "", fmt.Errorf("grpc-web: trailer frame missing grpc-status")
			}
			var c int
			if _, err := fmt.Sscanf(raw, "%d", &c); err != nil {
				return -1, "", fmt.Errorf("grpc-web: invalid grpc-status %q", raw)
			}
			return c, trailers["grpc-message"], nil
		}
	}
	if !foundTrailer {
		return -1, "", fmt.Errorf("grpc-web: no trailer frame in response")
	}
	return -1, "", fmt.Errorf("grpc-web: trailer frame without grpc-status")
}

func parseTrailerPayload(payload []byte) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(string(payload), "\r\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		result[strings.TrimSpace(strings.ToLower(parts[0]))] = strings.TrimSpace(parts[1])
	}
	return result
}

// ---------- Protobuf payloads ----------

// acceptTOSPayload is the fixed gRPC-Web encoded payload for SetTosAcceptedVersion.
// Proto: field 2, varint = 1 (true).
var acceptTOSPayload = grpcwebEncode([]byte{0x10, 0x01})

// buildNsfwPayload constructs the gRPC-Web encoded payload for UpdateUserFeatureControls.
// Sets always_show_nsfw_content to the given enabled value.
func buildNsfwPayload(enabled bool) []byte {
	name := []byte("always_show_nsfw_content")
	enableByte := byte(0x00)
	if enabled {
		enableByte = 0x01
	}
	// inner: field 1 (string) = name
	inner := append([]byte{0x0a, byte(len(name))}, name...)
	// protobuf: field 1 (embedded) = {field 2 (varint) = enabled}, field 2 (embedded) = inner
	protobuf := append([]byte{0x0a, 0x02, 0x10, enableByte, 0x12, byte(len(inner))}, inner...)
	return grpcwebEncode(protobuf)
}

// truncate returns the first n bytes of data as a string, appending "..." if truncated.
func truncate(data []byte, n int) string {
	if len(data) <= n {
		return string(data)
	}
	return string(data[:n]) + "..."
}
