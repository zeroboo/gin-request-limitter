package limitter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"strconv"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var dsClient *datastore.Client

const DatastoreKindRequestTracker = "test_request_trackers"
const FieldNameUserId string = "userId"

func TestMain(m *testing.M) {
	log.Println("[TestMain]")
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.TraceLevel)

	datastoreProjectId := os.Getenv("DATASTORE_PROJECT_ID")
	serviceAccount := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	//Init tests
	var errDatastore error
	dsClient, errDatastore = datastore.NewClient(context.Background(), datastoreProjectId, option.WithCredentialsFile(serviceAccount))
	log.Printf("Init datastore: projec tId=%v, error=%v, ", datastoreProjectId, errDatastore)
	CleanupTestData()
	//Run all tests
	exitCode := m.Run()

	os.Exit(exitCode)
}

func CleanupTestData() {
	ctx := context.Background()
	query := datastore.NewQuery(DatastoreKindRequestTracker)

	it := dsClient.Run(ctx, query)
	for {
		var tracker RequestTracker = RequestTracker{}
		key, errQuery := it.Next(tracker)
		if errQuery == iterator.Done {
			break
		}
		errDelete := dsClient.Delete(ctx, key)
		log.Printf("Delete key %v, error=%v", key, errDelete)
	}
}

func HandleHealth(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}

// CreateFakeAuthenticationHandler returns a handler accepts all request
func CreateFakeAuthenticationHandler(fieldNameUserId string, userIdValue string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(fieldNameUserId, userIdValue)
	}
}

// go test -timeout 30s -run ^TestLimitter_ValidGETRequest_Correct$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_ValidGETRequest_Correct(t *testing.T) {
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, "testUser"),
		CreateDatastoreBackedLimitter(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField(FieldNameUserId),
			200, 60000, 10),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "Response success")
	assert.Equal(t, "OK", recorder.Body.String(), "Response body OK")
}

var testHandlersSet []gin.HandlerFunc = []gin.HandlerFunc{
	CreateFakeAuthenticationHandler(FieldNameUserId, "testUser"),
	CreateDatastoreBackedLimitter(dsClient,
		DatastoreKindRequestTracker,
		GetUserIdFromContextByField("userId"),
		200, 60000, 10),
	HandleHealth,
}

// go test -timeout 30s -run ^TestLimitter_MultiRequestTooFast_ResponseError$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_MultiRequestTooFast_ResponseError(t *testing.T) {
	userId := "test-too-fast"
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitter(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField("userId"),
			200, 60000, 10),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "First response success")
	assert.Equal(t, "OK", recorder.Body.String(), "First response body OK")

	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitter(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField("userId"),
			200, 60000, 10),
		HandleHealth,
	)

	assert.Equal(t, http.StatusTooEarly, recorder2.Code, "Second response's code is too early ")
}

// go test -timeout 30s -run ^TestLimitter_MultiRequestNotTooFast_Success$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_MultiRequestNotTooFast_Success(t *testing.T) {
	userId := "test-too-fast"
	interval := int64(200)
	recorder := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitter(dsClient, DatastoreKindRequestTracker, GetUserIdFromContextByField("userId"),
			interval, 0, 0),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder.Code, "First response success")
	assert.Equal(t, "OK", recorder.Body.String(), "First response body OK")
	time.Sleep(time.Millisecond * time.Duration(interval+100))
	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		CreateFakeAuthenticationHandler(FieldNameUserId, userId),
		CreateDatastoreBackedLimitter(dsClient,
			DatastoreKindRequestTracker,
			GetUserIdFromContextByField("userId"),
			interval, 0, 0),
		HandleHealth,
	)

	assert.Equal(t, http.StatusOK, recorder2.Code, "Second response's code is success ")
}

// go test -timeout 30s -run ^TestLimitter_TooFrequentlyRequests_ResponseError$ github.com/zeroboo/gin-request-limitter -v
func TestLimitter_TooFrequentlyRequests_ResponseError(t *testing.T) {
	userId := "test-too-fast"
	interval := int64(10)
	limitter := CreateDatastoreBackedLimitter(dsClient, DatastoreKindRequestTracker,
		GetUserIdFromContextByField("userId"),
		interval, 10000, 1)
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
	limitter := CreateDatastoreBackedLimitter(dsClient, DatastoreKindRequestTracker,
		GetUserIdFromContextByField("userId"),
		interval, windowSize, 1)
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
	time.Sleep(time.Millisecond * time.Duration(interval+100))

	recorder2 := RecordRequest(http.MethodGet,
		"/health",
		map[string][]string{},
		map[string][]string{},
		authHandler,
		limitter,
		HandleHealth,
	)
	assert.Equal(t, http.StatusTooManyRequests, recorder2.Code, "Second response's code failed cause it too freequently")

	time.Sleep(time.Millisecond * time.Duration(windowSize*2))
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

// CreateRequest return a forged http request
func CreateRequest(method string, urlPath string, headers map[string][]string, params map[string][]string) *http.Request {
	requestParams := url.Values{}
	if len(params) > 0 {
		for paramKey, paramValues := range params {
			for _, paramValue := range paramValues {
				requestParams.Add(paramKey, paramValue)
			}
		}
	}
	payload := requestParams.Encode()

	req, _ := http.NewRequest(method, urlPath, strings.NewReader(payload))

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(payload)))
	for headerKey, headerValues := range headers {
		for _, headerValue := range headerValues {
			req.Header.Add(headerKey, headerValue)
		}
	}
	return req
}

func RecordRequest(method string, urlPath string, headers map[string][]string, params map[string][]string, handlers ...gin.HandlerFunc) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := gin.Default()

	r.Handle(method, urlPath, handlers...)
	req := CreateRequest(method, urlPath, headers, params)
	r.ServeHTTP(w, req)
	return w
}
