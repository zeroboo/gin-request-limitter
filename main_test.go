package limitter

import (
	"context"
	"math/rand"
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
)

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

func TestMain(m *testing.M) {
	log.Println("[TestMain]")
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.TraceLevel)

	datastoreProjectId := os.Getenv("DATASTORE_PROJECT_ID")
	serviceAccount := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	//Init datastore
	var errDatastore error
	dsClient, errDatastore = datastore.NewClient(context.Background(), datastoreProjectId, option.WithCredentialsFile(serviceAccount))
	log.Printf("Init datastore: projectId=%v, error=%v, ", datastoreProjectId, errDatastore)

	//Init random
	rand.Seed(time.Now().UnixNano())

	//Init redis
	InitRedis("test", "dev", "127.0.0.1:6379", "", 0)

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

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
