package limitter

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

var limitterTestConfig LimitterConfig = LimitterConfig{
	MinRequestInterval:  200,
	WindowSize:          60000,
	MaxRequestPerWindow: 10,
	ExpSec:              600,
}
var limitterTestConfigLongWindow LimitterConfig = LimitterConfig{
	MinRequestInterval:  200,
	WindowSize:          6000000,
	MaxRequestPerWindow: 2,
	ExpSec:              600,
}

var limitterTestConfigShortWindow LimitterConfig = LimitterConfig{
	MinRequestInterval:  0,
	WindowSize:          200,
	MaxRequestPerWindow: 1,
	ExpSec:              600,
}
var limitterHandlerShortWindow func(c *gin.Context) = CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfigShortWindow)
var limitterHandlerLongWindow func(c *gin.Context) = CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfigLongWindow)

// go.exe test -timeout 30s -run ^TestRedisLimitter_FirstRequest_HasError$ github.com/zeroboo/gin-request-limitter -v
func TestRedisLimitter_FirstRequest_HasError(t *testing.T) {
	userId := RandomString(16)
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfig),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "Response success")
	assert.Equal(t, "OK", recorder.Body.String(), "Response body OK")
}

// go.exe test -timeout 30s -run ^TestRedisLimitter_RequestTooFast_HasError$ github.com/zeroboo/gin-request-limitter -v
func TestRedisLimitter_RequestTooFast_HasError(t *testing.T) {
	userId := RandomString(16)
	RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfig),
		HandleHealth,
	)
	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfig),
		HandleHealth,
	)

	assert.Equal(t, http.StatusTooEarly, recorder2.Code, "Response has error code")
	assert.Equal(t, "", recorder2.Body.String(), "Response body empty")

}

// go.exe test -timeout 30s -run ^TestRedisLimitter_MultipleRequestsObeyInterval_NoError$ github.com/zeroboo/gin-request-limitter -v
func TestRedisLimitter_MultipleRequestsObeyInterval_NoError(t *testing.T) {
	userId := RandomString(16)
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfig),
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder.Code, "Response success")
	assert.Equal(t, "OK", recorder.Body.String(), "Response body success")

	time.Sleep(time.Duration(limitterTestConfig.MinRequestInterval+100) * time.Millisecond)
	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfig),
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder2.Code, "Response success")
	assert.Equal(t, "OK", recorder2.Body.String(), "Response body success")

	time.Sleep(time.Duration(limitterTestConfig.MinRequestInterval+100) * time.Millisecond)
	recorder3 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateRedisBackedLimitterFromConfig(GetUserIdFromContextByField(FieldNameUserId), &limitterTestConfig),
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder3.Code, "Response success")
	assert.Equal(t, "OK", recorder3.Body.String(), "Response body success")
}

// go.exe test -timeout 30s -run ^TestRedisLimitter_RequestTooFreequently_HasError$ github.com/zeroboo/gin-request-limitter -v
func TestRedisLimitter_RequestTooFreequently_HasError(t *testing.T) {
	userId := RandomString(16)

	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		limitterHandlerLongWindow,
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder.Code, "Response success")
	assert.Equal(t, "OK", recorder.Body.String(), "Response body success")

	time.Sleep(time.Duration(limitterTestConfigLongWindow.MinRequestInterval+100) * time.Millisecond)
	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		limitterHandlerLongWindow,
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder2.Code, "Response success")
	assert.Equal(t, "OK", recorder2.Body.String(), "Response body success")

	time.Sleep(time.Duration(limitterTestConfigLongWindow.MinRequestInterval+100) * time.Millisecond)
	recorder3 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		limitterHandlerLongWindow,
		HandleHealth,
	)
	assert.Equal(t, http.StatusTooManyRequests, recorder3.Code, "Response success")
	assert.Equal(t, "", recorder3.Body.String(), "Response body success")
}

// go.exe test -timeout 30s -run ^TestRedisLimitter_RequestTooFreequentlyAndWaitForNextWindow_Success$ github.com/zeroboo/gin-request-limitter -v
func TestRedisLimitter_RequestTooFreequentlyAndWaitForNextWindow_Success(t *testing.T) {
	userId := RandomString(16)

	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		limitterHandlerShortWindow,
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder.Code, "Response success")
	assert.Equal(t, "OK", recorder.Body.String(), "Response body success")

	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		limitterHandlerShortWindow,
		HandleHealth,
	)
	assert.Equal(t, http.StatusTooManyRequests, recorder2.Code, "Response too many request")
	assert.Equal(t, "", recorder2.Body.String(), "Response body empty")
	time.Sleep(time.Duration(limitterTestConfigShortWindow.WindowSize) * time.Millisecond)
	recorder3 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		limitterHandlerShortWindow,
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder3.Code, "Response success")
	assert.Equal(t, "OK", recorder3.Body.String(), "Response body success")
}
