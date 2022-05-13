package gohtmock

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type Mock struct {
	server                *httptest.Server
	callCount             map[string]int
	assertCallCountCalled map[string]bool
	mockResponses         []*mockResponse
	unmockedRequests      map[string]int
	sync.Mutex
}

func New() *Mock {
	m := &Mock{
		callCount:             make(map[string]int),
		assertCallCountCalled: make(map[string]bool),
		unmockedRequests:      make(map[string]int),
	}

	m.server = httptest.NewUnstartedServer(m)
	m.server.Start()
	return m
}

func (m *Mock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	path := r.URL.Path
	var mr *mockResponse
	m.Lock()
	for _, v := range m.mockResponses {
		if v.path == path && v.method == method {
			mr = v
			break
		}
	}
	m.Unlock()
	if mr == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s not found", path)
		m.unmockedRequests[method+path]++
		return
	}

	mr.Lock()
	for k, v := range mr.headers {
		w.Header().Set(k, v)
	}
	mr.Unlock()

	var status int
	if len(mr.callbacks) > 0 {
		status = mr.callbacks[m.callCount[method+path]](r)
	}

	m.Lock()
	m.callCount[method+path]++
	m.Unlock()
	if status != 0 {
		w.WriteHeader(status)
	}
	_, err := w.Write([]byte(mr.resp))
	if err != nil {
		log.Fatal("error writing respose for ", path, err)
	}
}

type mockResponse struct {
	resp      string
	path      string
	headers   map[string]string
	method    string
	httpMock  *Mock
	callbacks []func(*http.Request) int
	sync.Mutex
}

func (mr *mockResponse) SetHeader(key, value string) *mockResponse {
	mr.Lock()
	mr.headers[key] = value
	mr.Unlock()
	return mr
}
func (mr *mockResponse) SetMethod(method string) *mockResponse {
	mr.Lock()
	mr.method = method
	mr.Unlock()
	return mr
}

func (m *Mock) URL() string {
	return m.server.URL
}

func (m *Mock) Close() {
	m.server.Close()
}

func (m *Mock) Mock(path, resp string, callback ...func(*http.Request) int) *mockResponse {
	mr := &mockResponse{
		callbacks: callback,
		resp:      resp,
		path:      path,
		headers:   make(map[string]string),
		method:    "GET",
		httpMock:  m,
	}
	mr.headers["content-type"] = "application/json" // default here
	m.Lock()
	m.mockResponses = append(m.mockResponses, mr)
	m.Unlock()

	return mr
}

func (m *Mock) AssertCallCount(tb testing.TB, method, path string, expected int) {
	m.Lock()
	cnt, ok := m.callCount[method+path]
	if !ok {
		tb.Errorf("mocked but never called path: %s method: %s", path, method)
		m.Unlock()
		return
	}
	m.assertCallCountCalled[method+path] = true
	m.Unlock()
	assert.Equal(tb, expected, cnt, path)
}

func (m *Mock) AssertCallCountAsserted(tb testing.TB) {
	for url, cnt := range m.callCount {
		if _, ok := m.assertCallCountCalled[url]; !ok {
			tb.Errorf("url: %s is mocked but never asserted. It was called %d times", url, cnt)
		}
	}
}

func (m *Mock) AssertNoMissingMocks(tb testing.TB) {
	for url, cnt := range m.unmockedRequests {
		tb.Errorf("url: %s is called but not mocked. It was called %d times", url, cnt)
	}
}

func (m *Mock) AssertMocksCalled(tb testing.TB) {
	for _, mr := range m.mockResponses {
		if _, ok := m.callCount[mr.method+mr.path]; !ok {
			tb.Errorf("%s %s mocked but never called.", mr.method, mr.path)
		}
	}
}
