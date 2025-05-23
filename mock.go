package gohtmock

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type Mock struct {
	server           *httptest.Server
	mockResponses    []*mockResponse
	unmockedRequests map[string]int
	sync.Mutex
}

func New() *Mock {
	m := &Mock{
		unmockedRequests: make(map[string]int),
	}

	m.server = httptest.NewUnstartedServer(m)
	m.server.Start()
	return m
}

func (m *Mock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	path := r.URL.Path
	mapKey := method + " " + path
	var mr *mockResponse
	m.Lock()
	defer m.Unlock()

	var matches []*mockResponse
	var depleted []*mockResponse
	for _, v := range m.mockResponses {
		if v.path != path || v.method != method {
			continue
		}
		if v.isDepleted() {
			depleted = append(depleted, v)
			continue
		}
		matches = append(matches, v)
	}

	matches = m.withFiltersFirst(matches)

	for _, v := range matches {
		if v.checkFilter(r) {
			mr = v
			break
		}
	}

	if mr == nil && len(depleted) > 0 {
		log.Printf("No more mock responses available for %s %s; all have reached their call limit", method, path)
	}

	if mr == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s not found", path)
		m.unmockedRequests[mapKey]++
		return
	}

	for k, v := range mr.headers {
		w.Header().Set(k, v)
	}
	mr.callCount++

	if mr.responder != nil {
		mr.responder(w, r)
		return
	}

	var status int
	if len(mr.callbacks) > 0 {
		status = mr.callbacks[mr.callCount-1](r)
	}

	if status != 0 {
		w.WriteHeader(status)
	}
	_, err := w.Write([]byte(mr.resp))
	if err != nil {
		log.Fatal("error writing response for ", path, err)
	}
}

func (m *Mock) withFiltersFirst(responses []*mockResponse) []*mockResponse {
	slices.SortStableFunc(responses, func(a, b *mockResponse) int {
		if a.filter != nil && b.filter != nil {
			return 0
		}

		if a.filter != nil {
			return -1
		}

		if b.filter != nil {
			return 1
		}

		return 0
	})
	return responses
}

type mockResponse struct {
	resp      string
	path      string
	headers   map[string]string
	method    string
	httpMock  *Mock
	callbacks []func(*http.Request) int
	responder func(http.ResponseWriter, *http.Request)
	filter    func(*http.Request) bool
	callCount int
	maxcalls  int
	asserted  bool
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

// Filter sets a callback function to filter incoming HTTP requests.
// Only requests for which the callback returns true will match this mock response.
func (mr *mockResponse) Filter(callback func(*http.Request) bool) *mockResponse {
	mr.Lock()
	mr.filter = callback
	mr.Unlock()
	return mr
}

// Once sets the maximum number of times this mock response can be used to 1.
// This is useful for ensuring the mock is only matched a single time in tests.
func (mr *mockResponse) Once() {
	mr.maxcalls = 1
}

// Times sets the maximum number of times this mock response can be used.
// Use this to specify how many times the mock should match incoming requests.
func (mr *mockResponse) Times(n int) {
	mr.maxcalls = n
}

// AssertCallCount asserts that the mock response was called the expected number of times.
// It marks the mock as asserted and reports an error if the call count does not match.
func (mr *mockResponse) AssertCallCount(tb testing.TB, expected int) {
	mr.Lock()
	defer mr.Unlock()

	mr.asserted = true

	if mr.callCount == 0 {
		tb.Errorf("url: %s is mocked but never called. It was called %d times", mr.path, mr.callCount)
		return
	}
	assert.Equal(tb, expected, mr.callCount, fmt.Sprintf("url: %s expected to be called %d times. It was called %d times", mr.path, expected, mr.callCount))
}

func (mr *mockResponse) checkFilter(r *http.Request) bool {
	if mr.filter == nil {
		return true
	}
	return mr.filter(r)
}

func (mr *mockResponse) isDepleted() bool {
	mr.Lock()
	defer mr.Unlock()

	if mr.maxcalls != 0 && mr.callCount >= mr.maxcalls {
		return true
	}

	if len(mr.callbacks) == 0 {
		return false
	}

	return mr.callCount >= len(mr.callbacks)
}

func (m *Mock) URL() string {
	return m.server.URL
}

func (m *Mock) Close() {
	m.server.Close()
}

// Mock registers a new mock response for the given path and response body.
// Optionally, callback functions can be provided to handle the incoming *http.Request.
// If callback functions are provided, the mock will only match as many times as there are callbacks;
// each callback will be used once in order. After all callbacks are used, the mock will no longer match.
//
// Headers are defaulted to "Content-Type: application/json".
//
// Returns a pointer to the created mockResponse.
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

// MockFunc registers a new mock response for the given path using a custom responder function.
// The responder function receives the http.ResponseWriter and *http.Request for custom handling.
//
// Headers are defaulted to "Content-Type: application/json".
//
// Returns a pointer to the created mockResponse.
func (m *Mock) MockFunc(path string, responder func(http.ResponseWriter, *http.Request)) *mockResponse {
	mr := &mockResponse{
		responder: responder,
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

// AssertCallCount checks that all mocks for the given method and path was called the expected number of times in total.
// If the mocks was never called, it reports an error. Otherwise, it asserts that the call count matches the expected value.
func (m *Mock) AssertCallCount(tb testing.TB, method, path string, expected int) {
	m.Lock()
	defer m.Unlock()

	cnt := 0
	for _, mr := range m.mockResponses {
		if mr.path == path && mr.method == method {
			cnt += mr.callCount
			mr.asserted = true
		}
	}
	if cnt == 0 {
		tb.Errorf("mocked but never called path: %s method: %s", path, method)
		return
	}
	assert.Equal(tb, expected, cnt, fmt.Sprintf("url: %s %s expected to be called %d times. It was called %d times", method, path, expected, cnt))
}

// AssertCallCountAsserted checks that all mocked responses have been asserted with AssertCallCount.
// If any mock was called but not asserted, it reports an error.
func (m *Mock) AssertCallCountAsserted(tb testing.TB) {
	m.Lock()
	defer m.Unlock()

	for _, mr := range m.mockResponses {
		if !mr.asserted {
			tb.Errorf("url: %s is mocked but never asserted. It was called %d times", mr.path, mr.callCount)
			tb.Errorf(`create a mock with: .AssertCallCount(t, "%s", "%s", %d)`, mr.method, mr.path, mr.callCount)
		}
	}
}

// AssertNoMissingMocks checks if there were any requests made to URLs that were not mocked.
// If any such requests exist, it reports an error for each.
func (m *Mock) AssertNoMissingMocks(tb testing.TB) {
	m.Lock()
	defer m.Unlock()

	for request, cnt := range m.unmockedRequests {
		method := strings.Split(request, " ")[0]
		url := strings.Split(request, " ")[1]
		tb.Errorf("url: %s is called but not mocked. It was called %d times", request, cnt)
		if method == "GET" {
			tb.Errorf(`create a mock with: .Mock("%s", "response")`, url)
			continue

		}
		tb.Errorf(`create a mock with: .Mock("%s", "response").SetMethod("%s")`, url, method)

	}
}

// AssertMocksCalled checks that all mocked responses were actually called at least once or asserted.
// If any mock was never called and not asserted, it reports an error.
func (m *Mock) AssertMocksCalled(tb testing.TB) {
	m.Lock()
	defer m.Unlock()

	for _, mr := range m.mockResponses {
		if mr.callCount == 0 && !mr.asserted {
			tb.Errorf("%s %s mocked but never called.", mr.method, mr.path)
		}
	}
}
