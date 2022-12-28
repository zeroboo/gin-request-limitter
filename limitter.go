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
	AbortOnTrackerFailed bool

	SessionExpirationSeconds int64
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
	limitterConfig LimitterConfig) error {

	if limitterConfig.MinRequestInterval > 0 {
		if tracker.IsRequestTooFast(currentTime, limitterConfig.MinRequestInterval) {
			//log.Infof("InvalidRequest: TooFast, ID=%v, url=%v, IP=%v, elapse=%v", tracker.UID, requestURL, requestClientIP, currentTime.UnixMilli()-tracker.LastCall)
			return ErrorRequestTooFast
		}
	}

	//log.Printf("WindowSize=%v, callLimit=%v", limitterConfig.WindowSize, limitterConfig.WindowRequestMax)
	tracker.UpdateRequest(time.Now(), &limitterConfig)

	if limitterConfig.WindowSize > 0 {
		if tracker.IsRequestTooFrequently(currentTime, limitterConfig.MaxRequestPerWindow) {
			//log.Infof("InvalidRequest: TooMany, ID=%v, url=%v, IP=%v, window=%v, windowCount=%v", tracker.UID, requestURL, requestClientIP, tracker.Window, tracker.WindowCount)
			return ErrorRequestTooFreequently
		}
	}

	return nil
}

// processValidateResult aborts gin context if there is an error, let gin context run otherwise
func processValidateResult(validateError error, c *gin.Context) {
	if validateError == nil {
		c.Next()
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

	return trackerKey, &tracker, errTracker
}

const DefaultSessionExpirationSeconds int64 = 3600

func (config *LimitterConfig) CreateExpiration(Now time.Time) time.Time {
	var expSec int64 = DefaultSessionExpirationSeconds
	if config.SessionExpirationSeconds > 0 {
		expSec = config.SessionExpirationSeconds
	}

	return Now.Add(time.Duration(expSec) * time.Second)
}
func CreateDatastoreBackedLimitterFromConfig(client *datastore.Client, trackerKind string,
	getUserIdFromContext func(c *gin.Context) string,
	config *LimitterConfig) func(c *gin.Context) {
	return func(c *gin.Context) {
		// Sets the name/ID for the new entity.
		userId := getUserIdFromContext(c)
		url := c.Request.URL.Path

		//Load tracker
		trackerKey, tracker, errTracker := LoadUserTracker(client, trackerKind, url, userId)
		var errValidate error
		if errTracker != nil {
			if errTracker == datastore.ErrNoSuchEntity {
				//This error it not critical, bypass it
				errTracker = nil

				//Handle error: No session found, create new
				tracker = CreateNewRequestTracker(userId, url, config.CreateExpiration(time.Now()))

				if log.IsLevelEnabled(log.TraceLevel) {
					log.Tracef("RequestLimitter: TrackerNotFound, UID=%v, url=%v, key=%v", userId, url, trackerKey)
				}
			}
		}

		if errTracker == nil {
			// if log.IsLevelEnabled(log.TraceLevel) {
			// 	log.Tracef("RequestLimitter: LoadedTracker=%v", tracker)
			// }
			errValidate = ValidateRequest(tracker, time.Now(), url, c.ClientIP(), config)
		} else {
			//Error occur, log error and quit tracker
			log.Errorf("RequestLimitter: LoadTrackerFailed, UID=%v, url=%v, key=%v, error=%v", userId, url, trackerKey, errTracker)
			c.Next()
			return
		}

		var savedKey *datastore.Key
		if errValidate == nil {
			savedKey, errTracker = client.Put(context.Background(), trackerKey, tracker)
			if errTracker != nil {
				log.Errorf("RequestLimitter: UpdateTrackerFailed, UID=%v, key=%v, error=%v", userId, trackerKey, errTracker)
				c.Next()
				return
			}

		}
		processValidateResult(errValidate, c)

		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("RequestLimitter: ValidateFinish, UID=%v, url=%v, IP=%v, calls=%v|%v, window=%v/%v|%v, saved=%v, errValidate=%v, errTracker=%v",
				tracker.UID,
				url,
				c.ClientIP(),
				time.Now().UnixMilli()-tracker.LastCall, config.MinRequestInterval,
				tracker.WindowRequest, config.MaxRequestPerWindow, tracker.WindowNum,
				savedKey != nil,
				errValidate,
				errTracker,
			)
		}
	}
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
func CreateDatastoreBackedLimitter(client *datastore.Client,
	trackerKind string,
	getUserIdFromContext func(c *gin.Context) string,
	minRequestIntervalMilis int64,
	windowFrameMilis int64,
	maxRequestInWindow int,
	sessionExpirationSeconds int64) func(c *gin.Context) {
	config := LimitterConfig{
		MinRequestInterval:       minRequestIntervalMilis,
		WindowSize:               windowFrameMilis,
		MaxRequestPerWindow:      int64(maxRequestInWindow),
		SessionExpirationSeconds: sessionExpirationSeconds,
	}
	log.Infof("CreateDatastoreBackedLimitter: DatastoreKind=%v, minRequestInterval=%v, WindowsSize=%v, MaxRequestPerWindow=%v, SessionExpirationSeconds=%v",
		trackerKind,
		config.MinRequestInterval,
		config.WindowSize,
		config.MaxRequestPerWindow,
		config.SessionExpirationSeconds,
	)

	return CreateDatastoreBackedLimitterFromConfig(client, trackerKind, getUserIdFromContext, &config)
}

// ValidateGinRequest
func ValidateGinRequest(userId string, url string, c *gin.Context) {

}
