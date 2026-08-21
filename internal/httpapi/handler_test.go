package httpapi_test

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luisrpp/pc-control/internal/httpapi"
	"github.com/luisrpp/pc-control/internal/wake"
)

type recordingSender struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (s *recordingSender) Send() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

func (s *recordingSender) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type blockingSender struct {
	mu          sync.Mutex
	calls       int
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingSender() *blockingSender {
	return &blockingSender{started: make(chan struct{}), release: make(chan struct{})}
}

func (s *blockingSender) Send() error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	s.startedOnce.Do(func() { close(s.started) })
	<-s.release
	return nil
}

func (s *blockingSender) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *blockingSender) Release() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func newTestServer(t *testing.T, sender wake.Sender) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(httpapi.NewHandler(wake.New(sender)))
	t.Cleanup(server.Close)
	return server
}

func request(t *testing.T, server *httptest.Server, method, target string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+target, body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("HTTP request error = %v", err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func assertJSONContentType(t *testing.T, response *http.Response) {
	t.Helper()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		t.Errorf("invalid Content-Type %q: %v", response.Header.Get("Content-Type"), err)
		return
	}
	if mediaType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", response.Header.Get("Content-Type"))
	}
}

func assertErrorResponse(t *testing.T, response *http.Response, wantStatus int, wantCode string) []byte {
	t.Helper()
	if response.StatusCode != wantStatus {
		t.Errorf("status = %d, want %d", response.StatusCode, wantStatus)
	}
	assertJSONContentType(t, response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Errorf("response body = %q, want JSON error envelope: %v", body, err)
		return body
	}
	if envelope.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q", envelope.Error.Code, wantCode)
	}
	if envelope.Error.Message == "" {
		t.Error("error.message is empty, want concise human-readable message")
	}
	return body
}

func TestWakeCommandSuccess(t *testing.T) {
	for _, contentType := range []string{"", "application/json", "text/plain"} {
		t.Run("Content-Type="+contentType, func(t *testing.T) {
			sender := &recordingSender{}
			server := newTestServer(t, sender)
			response := request(t, server, http.MethodPost, "/v1/wake", nil, contentType)

			if response.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", response.StatusCode)
			}
			assertJSONContentType(t, response)
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("ReadAll(response body) error = %v", err)
			}
			if string(body) != `{"result":"sent"}` {
				t.Errorf("response body = %q, want %q", body, `{"result":"sent"}`)
			}
			if got := sender.Calls(); got != 1 {
				t.Errorf("sender calls = %d, want 1", got)
			}
		})
	}
}

func TestWakeCommandAcceptsTrailingQuestionMarkWithoutQueryContent(t *testing.T) {
	sender := &recordingSender{}
	server := newTestServer(t, sender)
	response := request(t, server, http.MethodPost, "/v1/wake?", nil, "")

	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	if got := sender.Calls(); got != 1 {
		t.Errorf("sender calls = %d, want 1", got)
	}
}

func TestWakeCommandRejectsQueryAndBodyInputWithoutSending(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		body        string
		contentType string
	}{
		{name: "named query", target: "/v1/wake?x=1"},
		{name: "empty query value", target: "/v1/wake?="},
		{name: "JSON body", target: "/v1/wake", body: "{}", contentType: "application/json"},
		{name: "plain body", target: "/v1/wake", body: "wake", contentType: "text/plain"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &recordingSender{}
			server := newTestServer(t, sender)
			response := request(t, server, http.MethodPost, test.target, strings.NewReader(test.body), test.contentType)

			assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
			if got := sender.Calls(); got != 0 {
				t.Errorf("sender calls = %d, want 0", got)
			}
		})
	}
}

func TestWakeCommandMethodHandlingPrecedesValidation(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			sender := &recordingSender{}
			server := newTestServer(t, sender)
			response := request(t, server, method, "/v1/wake?x=1", strings.NewReader("not empty"), "application/json")

			assertErrorResponse(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
			if got := response.Header.Get("Allow"); got != http.MethodPost {
				t.Errorf("Allow = %q, want %q", got, http.MethodPost)
			}
			if got := sender.Calls(); got != 0 {
				t.Errorf("sender calls = %d, want 0", got)
			}
		})
	}
}

func TestWakeCommandRejectsUnknownPathsWithoutSending(t *testing.T) {
	for _, target := range []string{"/v1/wake/", "/v1/unknown", "/"} {
		t.Run(target, func(t *testing.T) {
			sender := &recordingSender{}
			server := newTestServer(t, sender)
			response := request(t, server, http.MethodPost, target, nil, "")

			assertErrorResponse(t, response, http.StatusNotFound, "not_found")
			if got := sender.Calls(); got != 0 {
				t.Errorf("sender calls = %d, want 0", got)
			}
		})
	}
}

func TestWakeCommandMapsSendFailureWithoutLeakingInternalError(t *testing.T) {
	const internalSentinel = "INTERNAL-UDP-SOCKET-DETAIL"
	sender := &recordingSender{err: errors.New(internalSentinel)}
	server := newTestServer(t, sender)
	response := request(t, server, http.MethodPost, "/v1/wake", nil, "")

	body := assertErrorResponse(t, response, http.StatusServiceUnavailable, "wake_failed")
	if strings.Contains(string(body), internalSentinel) {
		t.Errorf("response body leaked internal error sentinel %q", internalSentinel)
	}
	if got := sender.Calls(); got != 1 {
		t.Errorf("sender calls = %d, want 1", got)
	}
}

func TestHEADNeverSendsAndHasNoBody(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       io.Reader
		wantStatus int
		wantAllow  string
	}{
		{name: "wake endpoint", target: "/v1/wake", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
		{name: "wake endpoint with query and body", target: "/v1/wake?x=1", body: strings.NewReader("body"), wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPost},
		{name: "unknown path", target: "/unknown", wantStatus: http.StatusNotFound},
		{name: "unknown path with query and body", target: "/unknown?x=1", body: strings.NewReader("body"), wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &recordingSender{}
			server := newTestServer(t, sender)
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
			if got := sender.Calls(); got != 0 {
				t.Errorf("sender calls = %d, want 0", got)
			}
		})
	}
}

func TestIncompleteRequestBodyDoesNotAcceptWakeCommand(t *testing.T) {
	sender := &recordingSender{}
	closed := make(chan struct{}, 1)
	testServer := httptest.NewUnstartedServer(httpapi.NewHandler(wake.New(sender)))
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
	if _, err := io.WriteString(connection, "POST /v1/wake HTTP/1.1\r\nHost: test\r\nContent-Length: 1\r\n\r\n"); err != nil {
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
	if got := sender.Calls(); got != 0 {
		t.Errorf("sender calls = %d, want 0 for incomplete request", got)
	}
}

func TestAcceptedCommandContinuesAfterClientDisconnect(t *testing.T) {
	sender := newBlockingSender()
	t.Cleanup(sender.Release)
	testServer := httptest.NewServer(httpapi.NewHandler(wake.New(sender)))
	t.Cleanup(testServer.Close)

	connection, err := net.Dial("tcp", strings.TrimPrefix(testServer.URL, "http://"))
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if _, err := io.WriteString(connection, "POST /v1/wake HTTP/1.1\r\nHost: test\r\nContent-Length: 0\r\n\r\n"); err != nil {
		t.Fatalf("write command error = %v", err)
	}

	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("wake sender did not begin after a complete valid request")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection error = %v", err)
	}
	if got := sender.Calls(); got != 1 {
		t.Errorf("sender calls after disconnect = %d, want 1", got)
	}

	sender.Release()
}
