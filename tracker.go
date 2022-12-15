package limitter

import "time"

// ----------------------------------------------------------------------------------------------------
type RequestTracker struct {
	UID    string
	URL    string
	Window int64

	//Calls of request in current window
	WindowCount int64
	//Last time request in millisec
	LastCall   int64
	Expiration time.Time
}

const DEFAULT_REQUEST_TRACKING_WINDOW_MILIS int64 = 60000
const SESSION_EXPIRATION_SECONDS int64 = 3600

func (tracker *RequestTracker) UpdateWindow(currentTime int64, windowMilis int64) {
	if windowMilis > 0 {
		currentWindow := currentTime / windowMilis
		if currentWindow > tracker.Window {
			tracker.Window = currentWindow
			tracker.WindowCount = 0
		}
	}
}

func (tracker *RequestTracker) UpdateRequest(currentTime time.Time, windowFrameMilis int64) {
	if windowFrameMilis > 0 {
		tracker.UpdateWindow(currentTime.UnixMilli(), windowFrameMilis)
		tracker.WindowCount += 1
	}

	tracker.LastCall = currentTime.UnixMilli()
	tracker.Expiration = time.Now().Add(time.Duration(SESSION_EXPIRATION_SECONDS) * time.Second)
}

func (tracker *RequestTracker) IsRequestTooFast(currentTime time.Time, requestMinIntervalMilis int64) bool {
	if tracker.LastCall == 0 {
		return false
	}
	return currentTime.UnixMilli()-tracker.LastCall < requestMinIntervalMilis
}

func (tracker *RequestTracker) IsRequestTooFrequently(currentTime time.Time, maxRequestPerWindow int64) bool {
	return tracker.WindowCount > maxRequestPerWindow
}
