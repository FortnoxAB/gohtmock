package gohtmock

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMock(t *testing.T) {
	a := 0
	mock := New()
	mock.Mock("/test", "ok")
	mock.Mock("/callback", "ok", func(*http.Request) int {
		a++
		return 201
	})

	resp, err := http.Get(mock.URL() + "/callback")
	assert.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	resp, err = http.Get(mock.URL() + "/test")
	assert.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "ok", string(body))
	mock.AssertCallCount(t, "GET", "/test", 1)
	mock.AssertCallCount(t, "GET", "/callback", 1)
	mock.AssertCallCountAsserted(t)
	mock.AssertNoMissingMocks(t)
	mock.AssertMocksCalled(t)
	assert.Equal(t, 1, a)
}

func TestMockMultipleResponseAndCallbacks(t *testing.T) {
	mock := New()
	mock.Mock("/test", "accepted", func(r *http.Request) int { return http.StatusAccepted })
	mock.Mock("/test", "ok", func(r *http.Request) int { return http.StatusOK })

	resp, err := http.Get(mock.URL() + "/test")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "accepted", string(body))

	resp, err = http.Get(mock.URL() + "/test")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "ok", string(body))
}

func TestMockResponder(t *testing.T) {
	mock := New()
	mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	resp, err := http.Get(mock.URL() + "/test")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "ok", string(body))
}

func TestMockResponderWithFilters(t *testing.T) {
	mock := New()
	// Should trigger for query id 1
	mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("user 1"))
	}).Filter(func(r *http.Request) bool {
		return r.URL.Query().Get("id") == "1"
	})

	// Should trigger as default fallback
	mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("user not found"))
	})

	// Should trigger for query id 2
	mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("user 2"))
	}).Filter(func(r *http.Request) bool {
		return r.URL.Query().Get("id") == "2"
	})

	assertBodyAndStatus(t, mock.URL()+"/test?id=1", "user 1", http.StatusOK)
	assertBodyAndStatus(t, mock.URL()+"/test?id=3", "user not found", http.StatusNotFound)
	assertBodyAndStatus(t, mock.URL()+"/test?id=2", "user 2", http.StatusOK)
	mock.AssertCallCount(t, http.MethodGet, "/test", 3)
	mock.AssertCallCountAsserted(t)
}

func TestMockResponderWithFiltersAsserts(t *testing.T) {
	mock := New()
	// Should trigger for query id 2
	mr3 := mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("user 2"))
	}).Filter(func(r *http.Request) bool {
		return r.URL.Query().Get("id") == "2"
	})

	// Should trigger for query id 1
	mr1 := mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("user 1"))
	}).Filter(func(r *http.Request) bool {
		return r.URL.Query().Get("id") == "1"
	})

	// Should trigger as default fallback
	mr2 := mock.MockFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("user not found"))
	})

	assertBodyAndStatus(t, mock.URL()+"/test?id=1", "user 1", http.StatusOK)
	assertBodyAndStatus(t, mock.URL()+"/test?id=3", "user not found", http.StatusNotFound)
	assertBodyAndStatus(t, mock.URL()+"/test?id=2", "user 2", http.StatusOK)
	mr1.AssertCallCount(t, 1)
	mr2.AssertCallCount(t, 1)
	mr3.AssertCallCount(t, 1)
	mock.AssertCallCountAsserted(t)
	mock.AssertNoMissingMocks(t)
	mock.AssertMocksCalled(t)
}

func TestAllMocksLimits(t *testing.T) {
	mock := New()
	mock.Mock("/test", "once").Once()
	mock.Mock("/test", "times").Times(2)

	assertBodyAndStatus(t, mock.URL()+"/test", "once", http.StatusOK)
	assertBodyAndStatus(t, mock.URL()+"/test", "times", http.StatusOK)
	assertBodyAndStatus(t, mock.URL()+"/test", "times", http.StatusOK)
	mock.AssertCallCount(t, "GET", "/test", 3)
	mock.AssertCallCountAsserted(t)
	mock.AssertNoMissingMocks(t)
	mock.AssertMocksCalled(t)
}

func TestNotAssertCallCount(t *testing.T) {
	mock := New()
	mock.Mock("/test", "ok")

	newT := &testing.T{}
	mock.AssertCallCount(newT, "GET", "/test", 1)
	assert.True(t, newT.Failed())
}

func TestNotAssertCallCountAsserted(t *testing.T) {
	mock := New()
	mock.Mock("/test", "ok")

	_, err := http.Get(mock.URL() + "/test")
	assert.NoError(t, err)

	newT := &testing.T{}
	mock.AssertCallCountAsserted(newT)
	assert.True(t, newT.Failed())
}

func TestNotAssertMocksCalled(t *testing.T) {
	mock := New()
	mock.Mock("/test", "ok")

	newT := &testing.T{}
	mock.AssertMocksCalled(newT)
	assert.True(t, newT.Failed())
}

func TestNotAssertNoMissingMocks(t *testing.T) {
	mock := New()
	_, err := http.Get(mock.URL() + "/test")
	assert.NoError(t, err)

	newT := &testing.T{}
	mock.AssertNoMissingMocks(newT)
	assert.True(t, newT.Failed())
}

func assertBodyAndStatus(t *testing.T, path, expBody string, expStatus int) bool {
	resp, err := http.Get(path)
	assert.NoError(t, err)
	assert.Equal(t, expStatus, resp.StatusCode, "Expected status %d but got %d", expStatus, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, expBody, string(body), "Expected body %s but got %s", expBody, string(body))

	return true
}
