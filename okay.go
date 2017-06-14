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

// New returns an empty OK, which is always valid but allows nothing and
// verifies nobody.
func New() OK {
	return nullOK{}
}

type nullOK struct{}

func (nullOK) Valid() bool                          { return true }
func (nullOK) Verify(context.Context) (bool, error) { return false, nil }
func (nullOK) Allows(interface{}) (bool, error)     { return false, nil }

type validOK struct {
	OK
	v func() bool
}

func (v *validOK) Valid() bool {
	return v.v() && v.OK.Valid()
}

// WithValid returns a new OK that will call the given function every time
// Valid() is called.  It is possible to attach many such functions by repeated
// application of this function.  All such functions must return true for
// Valid() to return true.
func WithValid(ok OK, valid func() bool) OK {
	return &validOK{
		OK: ok,
		v:  valid,
	}
}

// CancelFunc immediately marks the associated OK invalid.  Calls after the
// first have no effect.
type CancelFunc func()

// WithCancel returns an OK that will expire when CancelFunc is called.
func WithCancel(ok OK) (OK, CancelFunc) {
	var c int32
	return WithValid(ok, func() bool { return atomic.LoadInt32(&c) == 0 }), func() { atomic.StoreInt32(&c, 1) }
}

// Stubbed for testing.
var timeFunc = time.Now

// WithDeadline returns an OK that will expire once the deadline has passed.
func WithDeadline(ok OK, deadline time.Time) OK {
	return WithValid(ok, func() bool {
		return timeFunc().Before(deadline)
	})
}

// WithTimeout returns an OK that will expire after the given duration.
func WithTimeout(ok OK, timeout time.Duration) OK {
	exp := timeFunc().Add(timeout)
	return WithValid(ok, func() bool {
		return timeFunc().Before(exp)
	})
}
