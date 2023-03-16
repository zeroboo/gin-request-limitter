/*
Implementation of limitter in redis
*/

package limitter

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

var rdb *redis.Client
var environment string
var keyPrefix string

func InitRedis(pKeyPrefix string, pEnvironment string, pRedisServerAddress string, pRedisPassword string, pRedisDatabase int) {
	rdb = redis.NewClient(&redis.Options{
		Addr:     pRedisServerAddress,
		Password: pRedisPassword,
		DB:       pRedisDatabase, // use default DB
	})

	environment = pEnvironment
	keyPrefix = pKeyPrefix

	log.Infof("RedisRequestLimitter: Init, redisHost=%v, redisPass=%v, redisDB=%v, environment=%v, keyPrefix=%v",
		rdb.Options().Addr, len(rdb.Options().Password), rdb.Options().DB, environment, keyPrefix)
}

func CreateRedisTrackerKey(pUserId string, pUrl string) string {
	return fmt.Sprintf("%v:%v:%v:%v", keyPrefix, environment, pUserId, pUrl)
}

func LoadRedisRequestTracker(ctx context.Context, rClient *redis.Client, userId string, url string) (*RequestTracker, error) {
	trackerKey := CreateRedisTrackerKey(userId, url)
	var tracker *RequestTracker = NewRequestTracker(userId, url)
	errGetTracker := rClient.HGetAll(ctx, trackerKey).Scan(tracker)
	return tracker, errGetTracker
}

func SaveRedisRequestTracker(ctx context.Context, rClient *redis.Client, tracker *RequestTracker, expireSecond int64) error {
	_, errSetTracker := rdb.Pipelined(ctx, func(rdb redis.Pipeliner) error {
		trackerKey := CreateRedisTrackerKey(tracker.UID, tracker.URL)
		rdb.HSet(ctx, trackerKey, "uid", tracker.UID)
		rdb.HSet(ctx, trackerKey, "url", tracker.URL)
		rdb.HSet(ctx, trackerKey, "winNum", tracker.WindowNum)
		rdb.HSet(ctx, trackerKey, "winReq", tracker.WindowRequest)
		rdb.HSet(ctx, trackerKey, "last", tracker.LastCall)
		rdb.HSet(ctx, trackerKey, "exp", tracker.Exp)
		if expireSecond > 0 {
			rdb.Expire(ctx, trackerKey, time.Duration(expireSecond)*time.Second)
		}

		return nil
	})
	return errSetTracker
}

func CreateRedisBackedLimitterFromConfig(pUserIdExtractor func(c *gin.Context) string,
	pConfig *LimitterConfig) func(c *gin.Context) {

	return func(c *gin.Context) {

		// Sets the name/ID for the new entity.
		userId := pUserIdExtractor(c)
		url := c.Request.URL.Path
		currentTime := time.Now()

		//Validate too fast request
		trackerKey := CreateRedisTrackerKey(userId, url)

		tracker, errGetTracker := LoadRedisRequestTracker(c.Request.Context(), rdb, userId, url)
		if errGetTracker != nil {
			log.Errorf("RedisLimitter: userId=%v, url=%v, error=%v", userId, url, errGetTracker)
		}

		errValidate := ValidateRequest(tracker, currentTime, url, c.ClientIP(), pConfig)
		// if log.IsLevelEnabled(log.TraceLevel) {
		// 	log.Tracef("TrackerAfter: %v", tracker)
		// }

		errSetTracker := SaveRedisRequestTracker(c.Request.Context(), rdb, tracker, pConfig.ExpSec)
		if errValidate == nil {
			if errSetTracker != nil && pConfig.AbortOnFail {
				log.Errorf("RedisLimitter: SaveTrackerFailed, userId=%v, key=%v, error=%v", userId, trackerKey, errSetTracker)
				errValidate = errSetTracker
			}
		} else {
			log.Errorf("RedisLimitter: ValidateTrackerFailed, userId=%v, key=%v, error=%v", userId, trackerKey, tracker.LastCall, errValidate)
		}

		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("RequestLimitter: ValidateFinish, UID=%v, url=%v, IP=%v, calls=%v|%v, window=%v/%v|%v, key=%v, errValidate=%v",
				tracker.UID,
				url,
				c.ClientIP(),
				currentTime.UnixMilli()-tracker.LastCall, pConfig.MinRequestInterval,
				tracker.WindowRequest, pConfig.MaxRequestPerWindow, tracker.WindowNum,
				trackerKey,
				errValidate,
			)
		}

		ProcessValidateResult(errValidate, c)
	}
}
