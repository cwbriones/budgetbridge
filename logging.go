package main

import (
	"bytes"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type LoggingHTTPClient struct {
	logBody bool
	*http.Client
}

func (lhc *LoggingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	res, err := lhc.Client.Do(req)
	if err != nil {
		return nil, err
	}
	event := log.Debug().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Int("status", res.StatusCode)
	if lhc.logBody {
		res.Body = newLoggingBody(event, res.Body)
	} else {
		event.Msg("LoggingHTTPClient")
	}
	return res, err
}

type loggingBody struct {
	event *zerolog.Event
	buf   bytes.Buffer
	io.ReadCloser
}

func (l *loggingBody) Read(p []byte) (int, error) {
	n, err := l.ReadCloser.Read(p)
	if n > 0 {
		if n, _ := l.buf.Write(p[:n]); err != nil {
			return n, err
		}
	}
	return n, err
}

func (l *loggingBody) Close() error {
	l.event.
		Bytes("body", l.buf.Bytes()).
		Msg("")
	return l.ReadCloser.Close()
}

func newLoggingBody(event *zerolog.Event, inner io.ReadCloser) io.ReadCloser {
	return &loggingBody{
		event:      event,
		ReadCloser: inner,
	}
}
