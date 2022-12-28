package limitter

import "time"

// ----------------------------------------------------------------------------------------------------
type RequestTracker struct {
	UID string
	URL string

	//WindowNum is index of current window
	WindowNum int64

	//WindowRequest is calls of request in current window
	WindowRequest int64
	//Last time request in millisec
	LastCall int64

	//Expiration is time when this tracker expired
	Expiration time.Time
}

const DefaultREquestTrackingWindowMilis int64 = 60000

func CreateRequestTracker(uid string, url string, expiration time.Time) *RequestTracker {
	var tracker RequestTracker = RequestTracker{
		UID:           uid,
		URL:           url,
		Expiration:    expiration,
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

func (tracker *RequestTracker) UpdateRequest(currentTime time.Time, windowFrameMilis int64, expiration time.Time) {
	if windowFrameMilis > 0 {
		tracker.UpdateWindow(currentTime, windowFrameMilis)
		tracker.WindowRequest += 1
	}

	tracker.LastCall = currentTime.UnixMilli()
	tracker.Expiration = expiration
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
