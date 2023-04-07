/*
Limitter that uses datastore
*/

package limitter

import (
	"errors"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func CreateDatastoreBackedLimitter(pClient *datastore.Client, pTrackerKind string,
	pUserIdExtractor func(c *gin.Context) string,
	pConfig *LimitterConfig,
	pIsMiddleware bool) func(c *gin.Context) {

	return func(c *gin.Context) {
		// Sets the name/ID for the new entity.
		userId := pUserIdExtractor(c)
		url := c.Request.URL.Path
		currentTime := time.Now()
		tracker := &RequestTracker{}
		trackerName := CreateTrackerName(userId, url)
		trackerKey := datastore.NameKey(pTrackerKind, trackerName, nil)

		_, err := pClient.RunInTransaction(c.Request.Context(), func(tx *datastore.Transaction) error {

			errTracker := tx.Get(trackerKey, tracker)
			if errTracker != nil {
				_, isErrorFieldMismatch := errTracker.(*datastore.ErrFieldMismatch)
				if isErrorFieldMismatch {
					errTracker = nil
					if log.IsLevelEnabled(log.TraceLevel) {
						log.Tracef("LoadUserTracker: TypeMisMatch, kind=%v, url=%v, userId=%v, error=%v",
							pTrackerKind, url, userId, errTracker)
					}
				} else if errors.Is(errTracker, datastore.ErrNoSuchEntity) {
					errTracker = nil
					tracker = NewRequestTrackerWithExpiration(userId, url, pConfig.CreateExpiration(currentTime))
					if log.IsLevelEnabled(log.TraceLevel) {
						log.Tracef("LoadUserTracker: NotFound, kind=%v, url=%v, userId=%v, error=%v",
							pTrackerKind, url, userId, errTracker)
					}
				} else {
					//It's critical
					log.Errorf("LoadUserTracker: Failed, kind=%v, url=%v, userId=%v, error=%v",
						pTrackerKind, url, userId, errTracker)
					return errTracker
				}
			} else {
				if log.IsLevelEnabled(log.TraceLevel) {
					log.Tracef("RequestLimitter: TrackerLoaded, key=%v, tracker=%v", trackerKey, tracker)
				}
			}

			errValidate := ValidateRequest(tracker, currentTime, url, c.ClientIP(), pConfig)
			if errValidate != nil {
				return errValidate
			}

			_, errTracker = tx.Put(trackerKey, tracker)
			if errTracker != nil {
				log.Errorf("RequestLimitter: UpdateTrackerFailed, UID=%v, key=%v, error=%v", userId, trackerKey, errTracker)
				return errTracker
			}

			return nil
		})

		ProcessValidateResult(err, c, pIsMiddleware)

		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("RequestLimitter: ValidateFinish, UID=%v, url=%v, IP=%v, calls=%v|%v, window=%v/%v|%v, key=%v, errValidate=%v",
				tracker.UID,
				url,
				c.ClientIP(),
				currentTime.UnixMilli()-tracker.LastCall, pConfig.MinRequestInterval,
				tracker.WindowRequest, pConfig.MaxRequestPerWindow, tracker.WindowNum,
				trackerKey,
				err,
			)
		}
	}
}
