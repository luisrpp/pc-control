package httpapi_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luisrpp/pc-control/internal/httpapi"
	"github.com/luisrpp/pc-control/internal/status"
	"github.com/luisrpp/pc-control/internal/wake"
)

type recordingStatusProbe struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (p *recordingStatusProbe) Probe() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.err
}

func (p *recordingStatusProbe) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type blockingStatusProbe struct {
	recordingStatusProbe
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingStatusProbe() *blockingStatusProbe {
	return &blockingStatusProbe{started: make(chan struct{}), release: make(chan struct{})}
}

func (p *blockingStatusProbe) Probe() error {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.startedOnce.Do(func() { close(p.started) })
	<-p.release
	return nil
}

func (p *blockingStatusProbe) Release() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func newStatusTestServer(t *testing.T, probe status.Probe) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(httpapi.NewHandlerWithStatus(wake.New(&recordingSender{}), nil, status.New(probe)))
	t.Cleanup(server.Close)
	return server
}

func TestStatusReportsOnlineForSuccessfulProbe(t *testing.T) {
	probe := &recordingStatusProbe{}
	server := newStatusTestServer(t, probe)

	response := request(t, server, http.MethodGet, "/v1/status", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	assertJSONContentType(t, response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	if string(body) != `{"status":"online"}` {
		t.Errorf("response body = %q, want %q", body, `{"status":"online"}`)
	}
	if got := probe.Calls(); got != 1 {
		t.Errorf("probe calls = %d, want 1", got)
	}
}

func TestStatusReportsOfflineAsNormalResultWithoutLeakingProbeError(t *testing.T) {
	const internalSentinel = "INTERNAL-TCP-DIAL-DETAIL"
	probe := &recordingStatusProbe{err: errors.New(internalSentinel)}
	server := newStatusTestServer(t, probe)

	response := request(t, server, http.MethodGet, "/v1/status", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	assertJSONContentType(t, response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	if string(body) != `{"status":"offline"}` {
		t.Errorf("response body = %q, want %q", body, `{"status":"offline"}`)
	}
	if strings.Contains(string(body), internalSentinel) {
		t.Errorf("response body leaked internal probe sentinel %q", internalSentinel)
	}
	if got := probe.Calls(); got != 1 {
		t.Errorf("probe calls = %d, want 1; failed probes are normal offline results", got)
	}
}

func TestStatusAcceptsEmptyInputRegardlessOfContentType(t *testing.T) {
	for _, contentType := range []string{"", "application/json", "text/plain"} {
		t.Run("Content-Type="+contentType, func(t *testing.T) {
			probe := &recordingStatusProbe{}
			server := newStatusTestServer(t, probe)
			response := request(t, server, http.MethodGet, "/v1/status", nil, contentType)

			if response.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", response.StatusCode)
			}
			if got := probe.Calls(); got != 1 {
				t.Errorf("probe calls = %d, want 1", got)
			}
		})
	}
}

func TestStatusAcceptsTrailingQuestionMarkWithoutQueryContent(t *testing.T) {
	probe := &recordingStatusProbe{}
	server := newStatusTestServer(t, probe)
	response := request(t, server, http.MethodGet, "/v1/status?", nil, "")

	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	if got := probe.Calls(); got != 1 {
		t.Errorf("probe calls = %d, want 1", got)
	}
}

func TestStatusRejectsInvalidInputWithoutProbing(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		body        string
		contentType string
	}{
		{name: "named query", target: "/v1/status?x=1"},
		{name: "empty query value", target: "/v1/status?="},
		{name: "JSON body", target: "/v1/status", body: "{}", contentType: "application/json"},
		{name: "plain body", target: "/v1/status", body: "status", contentType: "text/plain"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &recordingStatusProbe{}
			server := newStatusTestServer(t, probe)
			response := request(t, server, http.MethodGet, test.target, strings.NewReader(test.body), test.contentType)

			assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
			if got := probe.Calls(); got != 0 {
				t.Errorf("probe calls = %d, want 0", got)
			}
		})
	}
}

func TestStatusMethodHandlingPrecedesValidation(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			probe := &recordingStatusProbe{}
			server := newStatusTestServer(t, probe)
			response := request(t, server, method, "/v1/status?x=1", strings.NewReader("not empty"), "application/json")

			assertErrorResponse(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
			if got := response.Header.Get("Allow"); got != http.MethodGet {
				t.Errorf("Allow = %q, want %q", got, http.MethodGet)
			}
			if got := probe.Calls(); got != 0 {
				t.Errorf("probe calls = %d, want 0", got)
			}
		})
	}
}

func TestStatusRejectsUnknownPathsWithoutProbing(t *testing.T) {
	for _, target := range []string{"/v1/status/", "/v1/unknown", "/"} {
		t.Run(target, func(t *testing.T) {
			probe := &recordingStatusProbe{}
			server := newStatusTestServer(t, probe)
			response := request(t, server, http.MethodGet, target, nil, "")

			assertErrorResponse(t, response, http.StatusNotFound, "not_found")
			if got := probe.Calls(); got != 0 {
				t.Errorf("probe calls = %d, want 0", got)
			}
		})
	}
}

func TestStatusHEADNeverProbesAndHasNoBody(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       io.Reader
		wantStatus int
		wantAllow  string
	}{
		{name: "status endpoint", target: "/v1/status", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "status endpoint with query and body", target: "/v1/status?x=1", body: strings.NewReader("body"), wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "unknown path", target: "/unknown", wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &recordingStatusProbe{}
			server := newStatusTestServer(t, probe)
			response := request(t, server, http.MethodHead, test.target, test.body, "text/plain")

			if response.StatusCode != test.wantStatus {
				t.Errorf("status = %d, want %d", response.StatusCode, test.wantStatus)
			}
			if got := response.Header.Get("Allow"); got != test.wantAllow {
				t.Errorf("Allow = %q, want %q", got, test.wantAllow)
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("ReadAll(response body) error = %v", err)
			}
			if len(body) != 0 {
				t.Errorf("HEAD response body = %q, want empty", body)
			}
			if got := probe.Calls(); got != 0 {
				t.Errorf("probe calls = %d, want 0", got)
			}
		})
	}
}

func TestIncompleteStatusRequestDoesNotProbe(t *testing.T) {
	probe := &recordingStatusProbe{}
	closed := make(chan struct{}, 1)
	testServer := httptest.NewUnstartedServer(httpapi.NewHandlerWithStatus(wake.New(&recordingSender{}), nil, status.New(probe)))
	testServer.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	testServer.Start()
	t.Cleanup(testServer.Close)

	connection, err := net.Dial("tcp", strings.TrimPrefix(testServer.URL, "http://"))
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if _, err := io.WriteString(connection, "GET /v1/status HTTP/1.1\r\nHost: test\r\nContent-Length: 1\r\n\r\n"); err != nil {
		t.Fatalf("write incomplete request error = %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection error = %v", err)
	}

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("server did not observe the incomplete request connection close")
	}
	if got := probe.Calls(); got != 0 {
		t.Errorf("probe calls = %d, want 0 for incomplete request", got)
	}
}

func TestAcceptedStatusContinuesAfterClientDisconnect(t *testing.T) {
	probe := newBlockingStatusProbe()
	defer probe.Release()
	testServer := httptest.NewServer(httpapi.NewHandlerWithStatus(wake.New(&recordingSender{}), nil, status.New(probe)))
	t.Cleanup(testServer.Close)

	connection, err := net.Dial("tcp", strings.TrimPrefix(testServer.URL, "http://"))
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if _, err := io.WriteString(connection, "GET /v1/status HTTP/1.1\r\nHost: test\r\nContent-Length: 0\r\n\r\n"); err != nil {
		t.Fatalf("write status request error = %v", err)
	}

	select {
	case <-probe.started:
	case <-time.After(time.Second):
		t.Fatal("status probe did not begin after a complete valid request")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection error = %v", err)
	}
	if got := probe.Calls(); got != 1 {
		t.Errorf("probe calls after disconnect = %d, want 1", got)
	}
}
