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

func CreateDatastoreBackedLimitterFromConfig(client *datastore.Client, trackerKind string,
	getUserIdFromContext func(c *gin.Context) string,
	config *LimitterConfig) func(c *gin.Context) {

	return func(c *gin.Context) {
		// Sets the name/ID for the new entity.
		userId := getUserIdFromContext(c)
		url := c.Request.URL.Path
		currentTime := time.Now()
		tracker := &RequestTracker{}
		trackerName := CreateTrackerName(userId, url)
		trackerKey := datastore.NameKey(trackerKind, trackerName, nil)

		_, err := client.RunInTransaction(c.Request.Context(), func(tx *datastore.Transaction) error {

			errTracker := tx.Get(trackerKey, tracker)
			if errTracker != nil {
				_, isErrorFieldMismatch := errTracker.(*datastore.ErrFieldMismatch)
				if isErrorFieldMismatch {
					errTracker = nil
					if log.IsLevelEnabled(log.TraceLevel) {
						log.Tracef("LoadUserTracker: TypeMisMatch, kind=%v, url=%v, userId=%v, error=%v",
							trackerKind, url, userId, errTracker)
					}
				} else if errors.Is(errTracker, datastore.ErrNoSuchEntity) {
					errTracker = nil
					tracker = CreateNewRequestTracker(userId, url, config.CreateExpiration(currentTime))
					if log.IsLevelEnabled(log.TraceLevel) {
						log.Tracef("LoadUserTracker: NotFound, kind=%v, url=%v, userId=%v, error=%v",
							trackerKind, url, userId, errTracker)
					}
				} else {
					//It's critical
					log.Errorf("LoadUserTracker: Failed, kind=%v, url=%v, userId=%v, error=%v",
						trackerKind, url, userId, errTracker)
					return errTracker
				}
			} else {
				if log.IsLevelEnabled(log.TraceLevel) {
					log.Tracef("RequestLimitter: TrackerLoaded, key=%v, tracker=%v", trackerKey, tracker)
				}
			}

			errValidate := ValidateRequest(tracker, currentTime, url, c.ClientIP(), config)
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

		processValidateResult(err, c)

		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("RequestLimitter: ValidateFinish, UID=%v, url=%v, IP=%v, calls=%v|%v, window=%v/%v|%v, key=%v, errValidate=%v",
				tracker.UID,
				url,
				c.ClientIP(),
				currentTime.UnixMilli()-tracker.LastCall, config.MinRequestInterval,
				tracker.WindowRequest, config.MaxRequestPerWindow, tracker.WindowNum,
				trackerKey,
				err,
			)
		}
	}
}
