package limitter

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Extract userId from a context by field "userId"
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

/*
Return 1 if request is valid, other values means invalid request
*/
func ValidateRequest(tracker *RequestTracker, currentTime time.Time, requestURL string, requestClientIP string, minRequestIntervalMilis int64) int {
	if tracker.IsRequestTooFast(currentTime, minRequestIntervalMilis) {
		log.Infof("InvalidRequest: TooFast, userId=%v, url=%v, IP=%v, elapse=%v", tracker.UserId, requestURL, requestClientIP, currentTime.UnixMilli()-tracker.LastCall)
		return VALIDATE_RESULT_TOO_FAST
	}
	if tracker.IsRequestTooFrequently(currentTime) {
		log.Infof("InvalidRequest: TooMany, userId=%v, url=%v, IP=%v, windowCount=%v", tracker.UserId, requestURL, requestClientIP, tracker.Count)
		return VALIDATE_RESULT_TOO_FREQUENTLY
	}

	return VALIDATE_RESULT_VALID
}

func ProcessValidateResult(validateResult int, c *gin.Context) {
	if validateResult == VALIDATE_RESULT_VALID {
		c.Next()
	} else if validateResult == VALIDATE_RESULT_TOO_FAST {
		c.AbortWithStatus(http.StatusTooEarly)
	} else if validateResult == VALIDATE_RESULT_TOO_FREQUENTLY {
		c.AbortWithStatus(http.StatusTooManyRequests)
	} else {
		c.AbortWithStatus(http.StatusInternalServerError)
	}
}
func HashKey(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	hashValue := hex.EncodeToString(h.Sum(nil))
	return hashValue
}
func CreateDatastoreBackedLimitter(client *datastore.Client, trackerKind string, GetUserIdFromContext func(c *gin.Context) string, minRequestIntervalMilis int64) func(c *gin.Context) {
	return func(c *gin.Context) {
		// Sets the name/ID for the new entity.
		userId := GetUserIdFromContext(c)
		url := c.Request.URL.Path
		// Creates a Key instance.
		keyRaw := fmt.Sprintf("%v|%v", userId, url)
		keyHash := HashKey(keyRaw)
		trackerKey := datastore.NameKey(trackerKind, keyHash, nil)
		//trackerKey := datastore.NameKey(trackerKind, keyRaw, nil)

		ctx := context.TODO()

		tracker := RequestTracker{}
		errTracker := client.Get(ctx, trackerKey, &tracker)
		validateResult := VALIDATE_RESULT_FAILED
		if errTracker != nil {
			if errTracker == datastore.ErrNoSuchEntity {
				//Handle error: No session found, create and save
				tracker.UserId = userId
				tracker.Url = url
				errTracker = nil
				log.Debugf("no tracker found: userId=%v, url=%v", userId, url)
			} else {
				log.Errorf("get tracker failed: userId=%v, key=%v, error=%v", userId, trackerKey, errTracker)
			}
		}

		if errTracker == nil {
			if log.IsLevelEnabled(log.DebugLevel) {
				log.Debugf("TrackerBefore: %v", tracker)
			}
			validateResult = ValidateRequest(&tracker, time.Now(), url, c.ClientIP(), MIN_REQUEST_INTERVAL_MILIS)
		}
		if log.IsLevelEnabled(log.DebugLevel) {
			log.Debugf("ValidateRequest: Done, userId=%v, url=%v, ip=%v, result=%v, errTracker=%v, elapse=%v", tracker.UserId, url, c.ClientIP(), validateResult, errTracker,
				time.Now().UnixMilli()-tracker.LastCall)
		}
		if validateResult == VALIDATE_RESULT_VALID {
			tracker.UpdateRequest(time.Now())
			var savedKey *datastore.Key
			savedKey, errTracker = client.Put(ctx, trackerKey, &tracker)
			if errTracker != nil {
				log.Infof("put tracker failed: userId=%v, key=%v, error=%v", userId, trackerKey, errTracker)
			}
			if log.IsLevelEnabled(log.DebugLevel) {
				log.Infof("put tracker done: userId=%v, key=%v, error=%v", userId, savedKey, errTracker)
			}
		}
		ProcessValidateResult(validateResult, c)
	}
}
