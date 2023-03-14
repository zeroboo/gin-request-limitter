package limitter

import (
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------------------------------
type RequestTracker struct {
	UID string `redis:"uid" datastore:"uid"`
	URL string `redis:"url" datastore:"url"`

	//WindowNum is index of current window
	WindowNum int64 `redis:"winNum" datastore:"winNum"`

	//WindowRequest is calls of request in current window
	WindowRequest int64 `redis:"winReq" datastore:"winReq"`
	//Last time request in millisec
	LastCall int64 `redis:"last" datastore:"last"`

	//Exp is expiration of this tracker as unix millisecond
	Exp int64 `redis:"exp" datastore:"exp"`
}

const DefaultRequestTrackingWindowMilis int64 = 60000

func NewRequestTrackerWithExpiration(uid string, url string, expiration time.Time) *RequestTracker {
	var tracker RequestTracker = RequestTracker{
		UID:           uid,
		URL:           url,
		Exp:           expiration.UnixMilli(),
		LastCall:      0,
		WindowNum:     0,
		WindowRequest: 0,
	}
	return &tracker
}
func NewRequestTracker(uid string, url string) *RequestTracker {
	var tracker RequestTracker = RequestTracker{
		UID:           uid,
		URL:           url,
		Exp:           0,
		LastCall:      0,
		WindowNum:     0,
		WindowRequest: 0,
	}
	return &tracker
}

func (tracker *RequestTracker) UpdateWindow(currentTime time.Time, windowMilis int64) {
	if windowMilis > 0 {
		currentWindow := currentTime.UnixMilli() / windowMilis
		if currentWindow != tracker.WindowNum {
			tracker.WindowNum = currentWindow
			tracker.WindowRequest = 0
		}
	}
}

func (tracker *RequestTracker) String() string {
	return fmt.Sprintf("UID:%v|URL:%v|Interval:%v|LastCall:%v|Window:%v:%v",
		tracker.UID,
		tracker.URL,
		time.Since(time.UnixMilli(tracker.LastCall)).Milliseconds(),
		tracker.LastCall,
		tracker.WindowNum, tracker.WindowRequest,
	)
}

func (tracker *RequestTracker) UpdateRequest(currentTime time.Time, config *LimitterConfig) {
	if config.WindowSize > 0 {
		tracker.UpdateWindow(currentTime, config.WindowSize)
		tracker.WindowRequest += 1
	}

	tracker.LastCall = currentTime.UnixMilli()
	tracker.Exp = config.CreateExpiration(currentTime).UnixMilli()
}

func (tracker *RequestTracker) IsRequestTooFast(currentTime time.Time, requestMinIntervalMilis int64) bool {
	if tracker.LastCall == 0 {
		return false
	}
	return currentTime.UnixMilli()-tracker.LastCall < requestMinIntervalMilis
}

func (tracker *RequestTracker) IsRequestTooFrequently(currentTime time.Time, maxRequestPerWindow int64) bool {
	return tracker.WindowRequest > maxRequestPerWindow
}
