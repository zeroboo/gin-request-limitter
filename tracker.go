package limitter

import "time"

// ----------------------------------------------------------------------------------------------------
type RequestTracker struct {
	UserId string
	Url    string
	Window int64

	//Calls of request in current window
	Count int64
	//Last time request in millisec
	LastCall int64
}

const REQUEST_TRACKING_WINDOW_MILIS int64 = 60000
const REQUEST_LIMIT_IN_WINDOW int64 = 10
const SESSION_EXPIRATION_SECONDS int64 = 24 * 3600

func (tracker *RequestTracker) UpdateWindow(currentTime int64) {
	currentWindow := currentTime / REQUEST_TRACKING_WINDOW_MILIS
	if currentWindow > tracker.Window {
		tracker.Window = currentWindow
		tracker.Count = 0
	}
}

func (tracker *RequestTracker) UpdateRequest(currentTime time.Time) {
	tracker.UpdateWindow(currentTime.Unix())
	tracker.Count += 1
	tracker.LastCall = currentTime.UnixMilli()
}

func (tracker *RequestTracker) IsRequestTooFast(currentTime time.Time, requestMinIntervalMilis int64) bool {
	if tracker.LastCall == 0 {
		return false
	}
	return currentTime.UnixMilli()-tracker.LastCall < requestMinIntervalMilis
}

func (tracker *RequestTracker) IsRequestTooFrequently(currentTime time.Time) bool {
	return tracker.Count > REQUEST_LIMIT_IN_WINDOW
}
