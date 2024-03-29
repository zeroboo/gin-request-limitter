package limitter

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// GetUserIdFromContextByField extracts userId from a gin context by property name
func GetUserIdFromContextByField(userIdField string) func(c *gin.Context) string {
	return func(c *gin.Context) string {
		return c.GetString(userIdField)
	}
}

const VALIDATE_RESULT_VALID int = 1
const VALIDATE_RESULT_TOO_FAST int = -1
const VALIDATE_RESULT_TOO_FREQUENTLY int = -2
const VALIDATE_RESULT_FAILED int = -3
const MIN_REQUEST_INTERVAL_MILIS int64 = 200

type LimitterConfig struct {
	//Time between 2 requests in milisecs. 0 means no limit
	MinRequestInterval int64

	//Window frame in milisec. Value 0 means no limit
	WindowSize int64

	//Max requests per window
	MaxRequestPerWindow int64

	//If true, error when save/load tracker will abort request
	//If false, request will be served even if save/load tracker error
	AbortOnFail bool

	//ExpSec is sesion expiration in seconds
	ExpSec int64
}

var ErrorRequestTooFast = fmt.Errorf("request is too fast")
var ErrorRequestTooFreequently = fmt.Errorf("request is too freequently")

/*
ValidateRequest returns nil if request is valid, an error means invalid request
*/
func ValidateRequest(tracker *RequestTracker,
	currentTime time.Time,
	requestURL string,
	requestClientIP string,
	limitterConfig *LimitterConfig) error {

	//log.Infof("ValidateRequest: Current=%v, lastCall=%v, passed=%v, minInterval=%v", currentTime.UnixMilli(), tracker.LastCall, currentTime.UnixMilli()-tracker.LastCall, limitterConfig.MinRequestInterval)
	if limitterConfig.MinRequestInterval > 0 {
		if tracker.IsRequestTooFast(currentTime, limitterConfig.MinRequestInterval) {
			//log.Infof("InvalidRequest: TooFast, ID=%v, url=%v, IP=%v, elapse=%v", tracker.UID, requestURL, requestClientIP, currentTime.UnixMilli()-tracker.LastCall)
			return ErrorRequestTooFast
		}
	}

	//log.Printf("WindowSize=%v, callLimit=%v", limitterConfig.WindowSize, limitterConfig.WindowRequestMax)
	tracker.UpdateRequest(currentTime, limitterConfig)

	if limitterConfig.WindowSize > 0 {
		if tracker.IsRequestTooFrequently(currentTime, limitterConfig.MaxRequestPerWindow) {
			//log.Infof("InvalidRequest: TooMany, ID=%v, url=%v, IP=%v, window=%v, windowCount=%v", tracker.UID, requestURL, requestClientIP, tracker.Window, tracker.WindowCount)
			return ErrorRequestTooFreequently
		}
	}

	return nil
}

// ProcessValidateResult aborts gin context if there is an error, let gin context run otherwise
func ProcessValidateResult(validateError error, c *gin.Context, isMiddleware bool) {
	if validateError == nil {
		if isMiddleware {
			c.Next()
		}

	} else if errors.Is(validateError, ErrorRequestTooFast) {
		c.AbortWithStatus(http.StatusTooEarly)
	} else if errors.Is(validateError, ErrorRequestTooFreequently) {
		c.AbortWithStatus(http.StatusTooManyRequests)
	} else {
		c.AbortWithStatus(http.StatusInternalServerError)
	}
}

// CreateTrackerName returns key of tracker based on userId and request URL.
// Key is a hash string to prevent invalid key in datastore
func CreateTrackerName(userId string, url string) string {
	keyRaw := fmt.Sprintf("%v|%v", userId, url)
	h := sha1.New()
	h.Write([]byte(keyRaw))
	hashValue := hex.EncodeToString(h.Sum(nil))
	return hashValue
}

// LoadUserTracker returns a tracker
func LoadUserTracker(Client *datastore.Client, TrackerKind string, URL string, UserId string) (*datastore.Key, *RequestTracker, error) {
	var tracker RequestTracker = RequestTracker{}
	trackerName := CreateTrackerName(UserId, URL)
	trackerKey := datastore.NameKey(TrackerKind, trackerName, nil)
	errTracker := Client.Get(context.TODO(), trackerKey, &tracker)

	if errTracker != nil {
		_, ok := errTracker.(*datastore.ErrFieldMismatch)
		if ok {
			errTracker = nil
			log.Warnf("LoadUserTracker: TypeMisMatch, kind=%v, url=%v, userId=%v, error=%v",
				TrackerKind, URL, UserId, errTracker)
		}
	}

	return trackerKey, &tracker, errTracker
}

const DefaultSessionExpirationSeconds int64 = 3600

func (config *LimitterConfig) CreateExpiration(Now time.Time) time.Time {
	var expSec int64 = DefaultSessionExpirationSeconds
	if config.ExpSec > 0 {
		expSec = config.ExpSec
	}

	return Now.Add(time.Duration(expSec) * time.Second)
}

/*
CreateDatastoreBackedLimitter returns a limitter with given config as middleware.

Limitter aborts gin context if validating failed.
If limitter has internal error, it will leaves the context run by calling c.Next()
Params:

  - trackerKind: Kind of tracker in datastore

  - GetUserIdFromContext: Function to extract userid from a gin context

  - minRequestIntervalMilis: Minimum time between 2 requests, 0 means no limit

  - windowFrameMilis: Window frame in miliseconds, 0 means no limit

  - maxRequestInWindow: Max request in a window frame
*/
func CreateDatastoreBackedLimitterHandler(client *datastore.Client,
	trackerKind string,
	getUserIdFromContext func(c *gin.Context) string,
	minRequestIntervalMilis int64,
	windowFrameMilis int64,
	maxRequestInWindow int,
	sessionExpirationSeconds int64) func(c *gin.Context) {
	config := LimitterConfig{
		MinRequestInterval:  minRequestIntervalMilis,
		WindowSize:          windowFrameMilis,
		MaxRequestPerWindow: int64(maxRequestInWindow),
		ExpSec:              sessionExpirationSeconds,
	}
	log.Infof("CreateDatastoreBackedLimitter: DatastoreKind=%v, minRequestIntervalMilis=%v, WindowsSize=%v, MaxRequestPerWindow=%v, SessionExpirationSeconds=%v",
		trackerKind,
		config.MinRequestInterval,
		config.WindowSize,
		config.MaxRequestPerWindow,
		config.ExpSec,
	)

	return CreateDatastoreBackedLimitter(client, trackerKind, getUserIdFromContext, &config, false)
}

/*
CreateDatastoreBackedLimitter returns a limitter with given config as middleware.

Limitter aborts gin context if validating failed.
If limitter has internal error, it will leaves the context run by calling c.Next()
Params:

  - trackerKind: Kind of tracker in datastore

  - GetUserIdFromContext: Function to extract userid from a gin context

  - minRequestIntervalMilis: Minimum time between 2 requests, 0 means no limit

  - windowFrameMilis: Window frame in miliseconds, 0 means no limit

  - maxRequestInWindow: Max request in a window frame
*/
func CreateDatastoreBackedLimitterMiddleware(client *datastore.Client,
	trackerKind string,
	getUserIdFromContext func(c *gin.Context) string,
	minRequestIntervalMilis int64,
	windowFrameMilis int64,
	maxRequestInWindow int,
	sessionExpirationSeconds int64) func(c *gin.Context) {
	config := LimitterConfig{
		MinRequestInterval:  minRequestIntervalMilis,
		WindowSize:          windowFrameMilis,
		MaxRequestPerWindow: int64(maxRequestInWindow),
		ExpSec:              sessionExpirationSeconds,
	}
	log.Infof("CreateDatastoreBackedLimitter: DatastoreKind=%v, minRequestIntervalMilis=%v, WindowsSize=%v, MaxRequestPerWindow=%v, SessionExpirationSeconds=%v",
		trackerKind,
		config.MinRequestInterval,
		config.WindowSize,
		config.MaxRequestPerWindow,
		config.ExpSec,
	)

	return CreateDatastoreBackedLimitter(client, trackerKind, getUserIdFromContext, &config, true)
}

func CreateRedisBackedLimitterHandler(getUserIdFromContext func(c *gin.Context) string,
	minRequestIntervalMilis int64,
	windowFrameMilis int64,
	maxRequestInWindow int,
	sessionExpirationSeconds int64) func(c *gin.Context) {
	config := LimitterConfig{
		MinRequestInterval:  minRequestIntervalMilis,
		WindowSize:          windowFrameMilis,
		MaxRequestPerWindow: int64(maxRequestInWindow),
		ExpSec:              sessionExpirationSeconds,
	}
	log.Infof("CreateRedisBackedLimitterHandler: minRequestIntervalMilis=%v, WindowsSize=%v, MaxRequestPerWindow=%v, SessionExpirationSeconds=%v",
		config.MinRequestInterval,
		config.WindowSize,
		config.MaxRequestPerWindow,
		config.ExpSec,
	)

	return CreateRedisBackedLimitter(getUserIdFromContext, &config, false)
}

func CreateRedisBackedLimitterMiddleware(getUserIdFromContext func(c *gin.Context) string,
	minRequestIntervalMilis int64,
	windowFrameMilis int64,
	maxRequestInWindow int,
	sessionExpirationSeconds int64) func(c *gin.Context) {
	config := LimitterConfig{
		MinRequestInterval:  minRequestIntervalMilis,
		WindowSize:          windowFrameMilis,
		MaxRequestPerWindow: int64(maxRequestInWindow),
		ExpSec:              sessionExpirationSeconds,
	}
	log.Infof("CreateRedisBackedLimitterMiddleware: minRequestIntervalMilis=%v, WindowsSize=%v, MaxRequestPerWindow=%v, SessionExpirationSeconds=%v",
		config.MinRequestInterval,
		config.WindowSize,
		config.MaxRequestPerWindow,
		config.ExpSec,
	)

	return CreateRedisBackedLimitter(getUserIdFromContext, &config, true)
}
