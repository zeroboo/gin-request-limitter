package limitter

import (
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/gin-gonic/gin"

	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var dsClient *datastore.Client

const DatastoreKindRequestTracker string = "test_request_trackers"
const FieldNameUserId string = "userId"

// go test -timeout 30s -run ^TestLimitter_ValidGETRequest_Correct$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_ValidGETRequest_Correct(t *testing.T) {
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, "testUser"),
		CreateDatastoreBackedLimitterHandler(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField(FieldNameUserId),
			200, 60000, 10, 3600*24),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "Response success")
	assert.Equal(t, "OK", recorder.Body.String(), "Response body OK")
}

var testHandlersSet []gin.HandlerFunc = []gin.HandlerFunc{
	CreateFakeAuthenticationHandler(FieldNameUserId, "testUser"),
	CreateDatastoreBackedLimitterHandler(dsClient,
		DatastoreKindRequestTracker,
		GetUserIdFromContextByField("userId"),
		200, 60000, 10, 3600*24),
	HandleHealth,
}

// go test -timeout 30s -run ^TestLimitter_MultiRequestTooFast_ResponseError$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_MultiRequestTooFast_ResponseError(t *testing.T) {
	userId := "test-too-fast"

	var minimumIntervalMilisecs int64 = 1000
	url := "/health"
	recorder := RecordRequest(http.MethodGet,
		url,
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitterHandler(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField("userId"),
			minimumIntervalMilisecs, 60000, 10, 3600*24),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "First response success")
	assert.Equal(t, "OK", recorder.Body.String(), "First response body OK")

	recorder2 := RecordRequest(http.MethodGet,
		url,
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitterHandler(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField("userId"),
			minimumIntervalMilisecs, 60000, 10, 3600*24),
		HandleHealth,
	)

	assert.Equal(t, http.StatusTooEarly, recorder2.Code, "Second response's code is too early ")

	//Test tracker
	key, tracker, err := LoadUserTracker(dsClient, DatastoreKindRequestTracker, url, userId)
	assert.Equal(t, err, nil, "Load tracker no error")
	assert.Equal(t, "/test_request_trackers,cda5c99c0242bc5b3a0ecf309c672d14b24683f0", fmt.Sprintf("%v", key), "Correct key")
	assert.Equal(t, url, tracker.URL, "Correct url")
	assert.Equal(t, int64(1), tracker.WindowRequest, "Correct calls")

}

// go test -timeout 30s -run ^TestLimitter_MultiRequestNotTooFast_Success$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_MultiRequestNotTooFast_Success(t *testing.T) {
	userId := fmt.Sprintf("test-too-fast-%v", time.Now().UnixMilli())
	interval := int64(200)
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitterHandler(dsClient, DatastoreKindRequestTracker, GetUserIdFromContextByField("userId"),
			interval, 0, 0, 3600*24),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "First response success")
	assert.Equal(t, "OK", recorder.Body.String(), "First response body OK")

	//Second request that respect interval restriction
	time.Sleep(time.Millisecond * time.Duration(interval+100))
	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitterHandler(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField("userId"),
			interval, 0, 0, 3600*24),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder2.Code, "Second response's code is success  ")
}

// go test -timeout 30s -run ^TestLimitter_TooFrequentlyRequests_ResponseError$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_TooFrequentlyRequests_ResponseError(t *testing.T) {
	userId := fmt.Sprintf("test-too-fast-%v", time.Now().Unix())
	interval := int64(10)
	limitter := CreateDatastoreBackedLimitterHandler(dsClient, DatastoreKindRequestTracker,
		GetUserIdFromContextByField("userId"),
		interval, 10000, 1, 3600*24)
	authHandler := CreateFakeAuthenticationHandler(FieldNameUserId, userId)
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		authHandler,
		limitter,
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "First response success")
	assert.Equal(t, "OK", recorder.Body.String(), "First response body OK")
	time.Sleep(time.Millisecond * time.Duration(interval+1))
	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		authHandler,
		limitter,
		HandleHealth,
	)
	assert.Equal(t, http.StatusTooManyRequests, recorder2.Code, "Second response's code failed cause it too freequently")
}

// go test -timeout 30s -run ^TestLimitterTooFreequently_NewWindow_RequestSuccess$ github.com/zeroboo/gin-request-limitter -v
func TestLimitterTooFreequently_NewWindow_RequestSuccess(t *testing.T) {
	userId := fmt.Sprintf("test-too-fast-%v", time.Now().Unix())
	interval := int64(10)
	windowSize := int64(1000)
	limitter := CreateDatastoreBackedLimitterHandler(dsClient, DatastoreKindRequestTracker,
		GetUserIdFromContextByField("userId"),
		interval, windowSize, 1, 3600*24)
	authHandler := CreateFakeAuthenticationHandler(FieldNameUserId, userId)

	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		authHandler,
		limitter,
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder.Code, "First response success")
	assert.Equal(t, "OK", recorder.Body.String(), "First response body OK")

	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		authHandler,
		limitter,
		HandleHealth,
	)
	assert.Equal(t, http.StatusTooManyRequests, recorder2.Code, "Second response's code failed cause it too freequently")

	//Sleep to next time window
	time.Sleep(time.Millisecond * time.Duration(windowSize+100))
	recorder3 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		authHandler,
		limitter,
		HandleHealth,
	)
	assert.Equal(t, http.StatusOK, recorder3.Code, "Third request as next window, must be success")
}

// go test -timeout 30s -run ^TestUpdateTracker_WindowsIncreased$ github.com/zeroboo/gin-request-limitter -v
func TestUpdateTracker_WindowsIncreased(t *testing.T) {
	var tracker *RequestTracker = NewRequestTrackerWithExpiration("uid", "url", time.Now().Add(100*time.Second))
	config := &LimitterConfig{}
	tracker.UpdateRequest(time.Now(), config)
	log.Infof("Tracker: %v", tracker)
}
