// Package okay defines an OK type, which can be used to gate access to
// arbitrary resources.
//
// An OK is composed of three elements, used for authentication, authorization,
// and expiration.
package okay

import (
	"context"
	"sync/atomic"
	"time"
)

// An OK represents both an authentication and an authorization guarding some
// resource.
type OK interface {
	// Valid reports whether this OK is still valid.  Once an OK has been marked
	// invalid (e.g. if it has been canceled) it must not become valid again.
	Valid() bool

	// Verify reports whether the given Context has a valid credential.  Verify
	// may return an error if there was an issue talking to any underlying
	// authentication mechanisms; if err is non-nil, ok must be false.
	Verify(context.Context) (ok bool, err error)

	// Allows reports whether this OK gates access to a given asset represented
	// by the argument, such as a file path.  If err is non-nil, ok must be
	// false.
	Allows(interface{}) (ok bool, err error)
}

// Base returns an empty OK, which is always valid but allows nothing and
// verifies nobody.
func Base() OK {
	return nullOK{}
}

type nullOK struct{}

func (nullOK) Valid() bool                          { return true }
func (nullOK) Verify(context.Context) (bool, error) { return false, nil }
func (nullOK) Allows(interface{}) (bool, error)     { return false, nil }

type cancelOK struct {
	OK
	c int32
}

func (c *cancelOK) Valid() bool {
	i := atomic.LoadInt32(&c.c)
	return i == 0 && c.OK.Valid()
}

// CancelFunc immediately marks the associated OK invalid.  Calls after the
// first have no effect.
type CancelFunc func()

// WithCancel returns an OK that will expire when CancelFunc is called.
func WithCancel(ok OK) (OK, CancelFunc) {
	c := &cancelOK{
		OK: ok,
	}
	return c, func() { atomic.StoreInt32(&c.c, 1) }
}

type expiresOK struct {
	OK
	exp time.Time
	now func() time.Time
}

func (e *expiresOK) Valid() bool {
	return e.now().Before(e.exp) && e.OK.Valid()
}

// Stubbed for testing.
var timeFunc = time.Now

// WithDeadline returns an OK that will expire once the deadline has passed.
func WithDeadline(ok OK, deadline time.Time) OK {
	return &expiresOK{
		OK:  ok,
		exp: deadline,
		now: timeFunc,
	}
}

// WithTimeout returns an OK that will expire after the given duration.
func WithTimeout(ok OK, timeout time.Duration) OK {
	return &expiresOK{
		OK:  ok,
		exp: timeFunc().Add(timeout),
		now: timeFunc,
	}
}
