package inference

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
)

type SSEEvent struct {
	Event string
	Data  []byte
}

// ReadSSE reads Server-Sent Events from r and calls onEvent for each complete event.
// Supports both named events ("event: x") and unnamed events (data-only).
func ReadSSE(r io.Reader, onEvent func(SSEEvent) error) error {
	br := bufio.NewReader(r)

	var eventName string
	var dataBuf bytes.Buffer

	flush := func() error {
		if eventName == "" && dataBuf.Len() == 0 {
			return nil
		}
		ev := SSEEvent{Event: eventName, Data: bytes.TrimSpace(dataBuf.Bytes())}
		eventName = ""
		dataBuf.Reset()
		return onEvent(ev)
	}

	for {
		line, err := br.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
		} else if strings.HasPrefix(line, ":") {
			// Comment/keepalive; ignore.
		} else if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(v)
		}

		if errors.Is(err, io.EOF) {
			return flush()
		}
	}
}

