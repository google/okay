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

package okay

import (
	"testing"
	"time"
)

func TestNullOK(t *testing.T) {
	ok := New()
	if !ok.Valid() {
		t.Errorf("New() is not valid")
	}
}
func fakeNow(before, after time.Time) (func() time.Time, func()) {
	var switched bool
	return func() time.Time {
			if switched {
				return after
			}
			return before
		}, func() {
			switched = true
		}
}
func TestDeadline(t *testing.T) {
	table := []struct {
		before      time.Time
		after       time.Time
		deadline    time.Time
		worksBefore bool
		worksAfter  bool
	}{
		{
			before:      time.Unix(10000, 0),
			after:       time.Unix(10010, 0),
			deadline:    time.Unix(10005, 0),
			worksBefore: true,
			worksAfter:  false,
		},
	}
	for _, ent := range table {
		now, swap := fakeNow(ent.before, ent.after)
		timeFunc = now
		ok := WithDeadline(New(), ent.deadline)
		if ok.Valid() != ent.worksBefore {
			t.Errorf("g.Valid() behaves unexpectedly at time %v with deadline %v: got %v, want %v", now(), ent.deadline, ok.Valid(), ent.worksBefore)
		}
		swap()
		if ok.Valid() != ent.worksAfter {
			t.Errorf("g.Valid() behaves unexpectedly at time %v with deadline %v: got %v, want %v", now(), ent.deadline, ok.Valid(), ent.worksAfter)
		}
	}
}
func TestTimeout(t *testing.T) {
	table := []struct {
		before      time.Time
		after       time.Time
		timeout     time.Duration
		worksBefore bool
		worksAfter  bool
	}{
		{
			before:      time.Unix(10000, 0),
			after:       time.Unix(10010, 0),
			timeout:     time.Second * 5,
			worksBefore: true,
			worksAfter:  false,
		},
	}
	for _, ent := range table {
		now, swap := fakeNow(ent.before, ent.after)
		timeFunc = now
		ok := WithTimeout(New(), ent.timeout)
		if ok.Valid() != ent.worksBefore {
			t.Errorf("g.Valid() behaves unexpectedly at time %v with timeout %v: got %v, want %v", now(), ent.timeout, ok.Valid(), ent.worksBefore)
		}
		swap()
		if ok.Valid() != ent.worksAfter {
			t.Errorf("g.Valid() behaves unexpectedly at time %v with timeout %v: got %v, want %v", now(), ent.timeout, ok.Valid(), ent.worksAfter)
		}
	}
}

func TestCancel(t *testing.T) {
	ok := New()
	if !ok.Valid() {
		t.Errorf("%T invalid", ok)
	}
	ok, cf := WithCancel(ok)
	if !ok.Valid() {
		t.Errorf("uncancelled OK invalid")
	}
	cf()
	if ok.Valid() {
		t.Errorf("cancelled OK is still valid")
	}
}
