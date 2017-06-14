// Copyright 2017 Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package okay defines an OK type, which can be used to gate access to
// arbitrary resources.
//
// An OK is composed of three elements, used for authentication, authorization,
// and expiration.
package okay

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// An OK represents both an authentication and an authorization guarding some
// resource.
type OK interface {
	// Valid reports whether this OK is still valid.  Once an OK has been marked
	// invalid (e.g. if it has been canceled) it must not become valid again.
	Valid() bool

	// Verify reports whether the given Context has a valid credential.  If the
	// given context has invalid credentials.  ok must be false if either err is
	// non-nil, but it is valid for ok to be false when err is nil.
	Verify(ctx context.Context) (ok bool, err error)

	// Allows reports whether this OK gates access to a given asset represented
	// by the argument, such as a file path.  If err is non-nil, ok must be
	// false, but ok may be false while err is nil.
	Allows(resource interface{}) (ok bool, err error)
}

var Invalid = errors.New("OK not valid")

// New returns an empty OK, which is always valid but allows nothing and
// verifies nobody.
func New() OK {
	return nullOK{}
}

func check(ctx context.Context, resource interface{}, ok OK) (bool, error) {
	if !ok.Valid() {
		return false, Invalid
	}
	if k, err := ok.Verify(ctx); !k {
		return false, err
	}
	return ok.Allows(resource)
}

// Check returns true if the given OKs are valid, verify the context, and allow
// the resource.  It returns true if *any* of the passed OKs is valid.
func Check(ctx context.Context, resource interface{}, ok ...OK) (bool, error) {
	var e error
	for _, ok := range ok {
		v, err := check(ctx, resource, ok)
		if v {
			return true, nil
		}
		if err != nil && (e != nil || e == Invalid) {
			e = err
		}
	}
	return false, e
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

// Validate returns a new OK that will call the given function every time
// Valid() is called.  It is possible to attach many such functions by repeated
// application of this function.  All such functions must return true for
// Valid() to return true.
func Validate(ok OK, valid func() bool) OK {
	return &validOK{
		OK: ok,
		v:  valid,
	}
}

type verifyOK struct {
	OK
	v func(context.Context) (bool, error)
}

func (v *verifyOK) Verify(ctx context.Context) (bool, error) {
	k, err := v.v(ctx)
	if k {
		return true, nil
	}
	if err != nil {
		if k, _ := v.OK.Verify(ctx); k {
			return true, nil
		}
		return false, err
	}
	return v.OK.Verify(ctx)
}

// Verify returns a new OK that will call the given function when OK.Verify()
// is called.  It is possible to attach multiple such functions by repeated
// calls to this function.  Functions are called in reverse order.  The first
// function to return valid=true will end the call chain and Valid() will
// return (true, nil).  The first function to return a non-nil err will have
// that error returned if no subsequent function returns true.  If *any* verify
// function returns true, Verify() will return (true, nil).
func Verify(ok OK, verify func(context.Context) (valid bool, err error)) OK {
	return &verifyOK{
		OK: ok,
		v:  verify,
	}
}

type allowOK struct {
	OK
	a func(interface{}) (bool, error)
}

func (a *allowOK) Allows(i interface{}) (bool, error) {
	ok, err := a.a(i)
	if ok {
		return true, nil
	}
	if err != nil {
		if ok, _ := a.OK.Allows(i); ok {
			return true, nil
		}
		return false, err
	}
	return a.OK.Allows(i)
}

// Allow returns an OK that calls the provided function whenever OK.Allow() is
// called.  Multiple such functions may be attached by successive calls to this
// function.  The functions are called in reverse order.  If *any* such
// function returns allowed=true,  Allow() will return (true, nil).  The first
// function to return a non-nil error will have that error returned if no
// function returns true.  It is valid for all functions to return (false,
// nil).
func Allow(ok OK, allow func(resource interface{}) (allowed bool, err error)) OK {
	return &allowOK{
		OK: ok,
		a:  allow,
	}
}

// CancelFunc immediately marks the associated OK invalid.  Calls after the
// first have no effect.
type CancelFunc func()

// WithCancel returns an OK that will expire when CancelFunc is called.
func WithCancel(ok OK) (OK, CancelFunc) {
	var c int32
	return Validate(ok, func() bool { return atomic.LoadInt32(&c) == 0 }), func() { atomic.StoreInt32(&c, 1) }
}

// WithContext returns an OK that will expire when the context is canceled.
func WithContext(ok OK, ctx context.Context) OK {
	return Validate(ok, func() bool { return ctx.Err() == nil })
}

// Stubbed for testing.
var timeFunc = time.Now

// WithDeadline returns an OK that will expire once the deadline has passed.
func WithDeadline(ok OK, deadline time.Time) OK {
	return Validate(ok, func() bool {
		return timeFunc().Before(deadline)
	})
}

// WithTimeout returns an OK that will expire after the given duration.
func WithTimeout(ok OK, timeout time.Duration) OK {
	exp := timeFunc().Add(timeout)
	return Validate(ok, func() bool {
		return timeFunc().Before(exp)
	})
}
