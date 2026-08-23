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
	"github.com/luisrpp/pc-control/internal/shutdown"
	"github.com/luisrpp/pc-control/internal/wake"
)

type recordingShutdownPort struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (p *recordingShutdownPort) Shutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.err
}

func (p *recordingShutdownPort) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type blockingShutdownPort struct {
	mu          sync.Mutex
	calls       int
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingShutdownPort() *blockingShutdownPort {
	return &blockingShutdownPort{started: make(chan struct{}), release: make(chan struct{})}
}

func (p *blockingShutdownPort) Shutdown() error {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.startedOnce.Do(func() { close(p.started) })
	<-p.release
	return nil
}

func (p *blockingShutdownPort) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *blockingShutdownPort) Release() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func newShutdownTestServer(t *testing.T, port shutdown.Port) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(httpapi.NewHandler(wake.New(&recordingSender{}), shutdown.New(port)))
	t.Cleanup(server.Close)
	return server
}

func TestShutdownCommandSuccess(t *testing.T) {
	for _, contentType := range []string{"", "application/json", "text/plain"} {
		t.Run("Content-Type="+contentType, func(t *testing.T) {
			port := &recordingShutdownPort{}
			server := newShutdownTestServer(t, port)
			response := request(t, server, http.MethodPost, "/v1/shutdown", nil, contentType)

			if response.StatusCode != http.StatusAccepted {
				t.Errorf("status = %d, want 202", response.StatusCode)
			}
			assertJSONContentType(t, response)
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("ReadAll(response body) error = %v", err)
			}
			if string(body) != `{"result":"initiated"}` {
				t.Errorf("response body = %q, want %q", body, `{"result":"initiated"}`)
			}
			if got := port.Calls(); got != 1 {
				t.Errorf("shutdown port calls = %d, want 1", got)
			}
		})
	}
}

func TestShutdownCommandAcceptsTrailingQuestionMarkWithoutQueryContent(t *testing.T) {
	port := &recordingShutdownPort{}
	server := newShutdownTestServer(t, port)
	response := request(t, server, http.MethodPost, "/v1/shutdown?", nil, "")

	if response.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", response.StatusCode)
	}
	if got := port.Calls(); got != 1 {
		t.Errorf("shutdown port calls = %d, want 1", got)
	}
}

func TestShutdownCommandRejectsQueryAndBodyInputWithoutOperation(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		body        string
		contentType string
	}{
		{name: "named query", target: "/v1/shutdown?x=1"},
		{name: "empty query value", target: "/v1/shutdown?="},
		{name: "JSON body", target: "/v1/shutdown", body: "{}", contentType: "application/json"},
		{name: "plain body", target: "/v1/shutdown", body: "shutdown", contentType: "text/plain"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port := &recordingShutdownPort{}
			server := newShutdownTestServer(t, port)
			response := request(t, server, http.MethodPost, test.target, strings.NewReader(test.body), test.contentType)

			assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
			if got := port.Calls(); got != 0 {
				t.Errorf("shutdown port calls = %d, want 0", got)
			}
		})
	}
}

func TestShutdownCommandMethodHandlingPrecedesValidation(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			port := &recordingShutdownPort{}
			server := newShutdownTestServer(t, port)
			response := request(t, server, method, "/v1/shutdown?x=1", strings.NewReader("not empty"), "application/json")

			assertErrorResponse(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
			if got := response.Header.Get("Allow"); got != http.MethodPost {
				t.Errorf("Allow = %q, want %q", got, http.MethodPost)
			}
			if got := port.Calls(); got != 0 {
				t.Errorf("shutdown port calls = %d, want 0", got)
			}
		})
	}
}

func TestShutdownCommandRejectsUnknownPathsWithoutOperation(t *testing.T) {
	for _, target := range []string{"/v1/shutdown/", "/v1/unknown", "/"} {
		t.Run(target, func(t *testing.T) {
			port := &recordingShutdownPort{}
			server := newShutdownTestServer(t, port)
			response := request(t, server, http.MethodPost, target, nil, "")

			assertErrorResponse(t, response, http.StatusNotFound, "not_found")
			if got := port.Calls(); got != 0 {
				t.Errorf("shutdown port calls = %d, want 0", got)
			}
		})
	}
}

func TestShutdownCommandMapsFailureWithoutLeakingInternalError(t *testing.T) {
	const internalSentinel = "INTERNAL-SSH-ERROR-DETAIL"
	port := &recordingShutdownPort{err: errors.New(internalSentinel)}
	server := newShutdownTestServer(t, port)
	response := request(t, server, http.MethodPost, "/v1/shutdown", nil, "")

	body := assertErrorResponse(t, response, http.StatusServiceUnavailable, "shutdown_failed")
	if strings.Contains(string(body), internalSentinel) {
		t.Errorf("response body leaked internal error sentinel %q", internalSentinel)
	}
	if got := port.Calls(); got != 1 {
		t.Errorf("shutdown port calls = %d, want 1", got)
	}
}

func TestShutdownHEADNeverOperatesAndHasNoBody(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       io.Reader
		wantStatus int
		wantAllow  string
	}{
		{name: "shutdown endpoint", target: "/v1/shutdown", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
		{name: "shutdown endpoint with query and body", target: "/v1/shutdown?x=1", body: strings.NewReader("body"), wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
		{name: "unknown path", target: "/unknown", wantStatus: http.StatusNotFound},
		{name: "unknown path with query and body", target: "/unknown?x=1", body: strings.NewReader("body"), wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port := &recordingShutdownPort{}
			server := newShutdownTestServer(t, port)
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
			if got := port.Calls(); got != 0 {
				t.Errorf("shutdown port calls = %d, want 0", got)
			}
		})
	}
}

func TestIncompleteRequestBodyDoesNotAcceptShutdownCommand(t *testing.T) {
	port := &recordingShutdownPort{}
	closed := make(chan struct{}, 1)
	testServer := httptest.NewUnstartedServer(httpapi.NewHandler(wake.New(&recordingSender{}), shutdown.New(port)))
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
	if _, err := io.WriteString(connection, "POST /v1/shutdown HTTP/1.1\r\nHost: test\r\nContent-Length: 1\r\n\r\n"); err != nil {
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
	if got := port.Calls(); got != 0 {
		t.Errorf("shutdown port calls = %d, want 0 for incomplete request", got)
	}
}

func TestAcceptedShutdownContinuesAfterClientDisconnect(t *testing.T) {
	port := newBlockingShutdownPort()
	t.Cleanup(port.Release)
	testServer := httptest.NewServer(httpapi.NewHandler(wake.New(&recordingSender{}), shutdown.New(port)))
	t.Cleanup(testServer.Close)

	connection, err := net.Dial("tcp", strings.TrimPrefix(testServer.URL, "http://"))
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if _, err := io.WriteString(connection, "POST /v1/shutdown HTTP/1.1\r\nHost: test\r\nContent-Length: 0\r\n\r\n"); err != nil {
		t.Fatalf("write command error = %v", err)
	}

	select {
	case <-port.started:
	case <-time.After(time.Second):
		t.Fatal("shutdown port did not begin after a complete valid request")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection error = %v", err)
	}
	if got := port.Calls(); got != 1 {
		t.Errorf("shutdown port calls after disconnect = %d, want 1", got)
	}

	port.Release()
}

func TestWakeBehaviorIsUnchangedWhenShutdownUseCaseIsPresent(t *testing.T) {
	sender := &recordingSender{}
	port := &recordingShutdownPort{}
	server := httptest.NewServer(httpapi.NewHandler(wake.New(sender), shutdown.New(port)))
	t.Cleanup(server.Close)

	response := request(t, server, http.MethodPost, "/v1/wake", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	if string(body) != `{"result":"sent"}` {
		t.Errorf("response body = %q, want %q", body, `{"result":"sent"}`)
	}
	if got := sender.Calls(); got != 1 {
		t.Errorf("wake sender calls = %d, want 1", got)
	}
	if got := port.Calls(); got != 0 {
		t.Errorf("shutdown port calls = %d, want 0", got)
	}
}
