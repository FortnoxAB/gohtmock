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
