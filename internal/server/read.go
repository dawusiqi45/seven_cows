package server

import (
	"fmt"
	"io"
	"net/http"
)

func readAllLimited(r *http.Request, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("payload is larger than %d bytes", maxBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("audio payload is empty")
	}
	return data, nil
}
