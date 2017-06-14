# okay: abstract cross-package auth[nz]

okay is a package that allows consumers to pass authentication and
authorization checks across package boundaries.

Consumers should start with the base OK, which denies everyone to everything,
and add permissions as required with the `Allow`, `Validate`, and `Verify`
functions.

## Overview

Often there is some resource to which we want to restrict access; databases (or
columns, or rows), file systems, URL endpoints, or even specific functions or
methods.

Say we have a file system type:

```go
package fs

import (
	"io"
	"os"
	"path/filepath"
)

type FileSystem struct {
	Root string
}

func (fs FileSystem) Open(path string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(fs.Root, path))
}
```

This could be used to e.g. serve the file via HTTP, but it provides no access
controls.  We could add this by modifying Open to take a Context which contains
the appropriate credentials:

```go
import (
	"context"
	"io"
	"os"
	"path/filepath"
)

func (fs FileSystem) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	// TODO: check context somehow
	return os.Open(filepath.Join(fs.Root, path))
}
```

How should we check the context?  If we're serving this via HTTP, we could
provide simple authentication by wrapping HTTP basic authentication into a
context:

```go
package server

import (
	"context"
	"net/http"
)

type authType string

func getContext(r *http.Request) context.Context {
	ctx := r.Context()
	if user, pass, ok := r.BasicAuth(); ok {
		ctx = context.WithValue(ctx, authType("user"), user)
		ctx = context.WithValue(ctx, authType("pass"), pass)
	}
	return ctx
}

func handleRequest(rw http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)
	doTheThing(ctx, rw)
}
```

...and then providing a facility to verify that context:

```go
package server

import "context"

func (s *Server) Authenticate(ctx context.Context) (bool, error) {
	user, ok := ctx.Value(authType("user")).(string)
	if !ok {
		return false, nil
	}
	pass, ok := ctx.Value(authType("pass")).(string)
	if !ok {
		return false, nil
	}

  return s.checkCredentials(user, pass)
}
```

Now we know how to check the context and can successfully gate access to our
files.

```go
import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"server"
)

type FileSystem struct {
	Server server.Server
	Root   string
}

func (fs *FileSystem) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	ok, err := fs.Server.Authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s: permission denied", path)
	}
	return os.Open(filepath.Join(fs.Root, path))
}
```

Unfortunately, this very tightly couples packages `fs` and `server`.  If `fs`
wants to grant users via another scheme, such as OAuth2, or to certain
administrator users, or in some other way, all the methods which check for
authentication must be updated.  It may be that `server` is not something we
can modify; it may be that `fs` isn't something we can modify.

This can be fixed by having `fs` accept OK types:

```go
import (
	"context"
	"fmt"
	"io"
	"okay"
	"os"
	"path/filepath"
)

type FileSystem struct {
	Auths []okay.OK
	Root  string
}

func (fs FileSystem) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	ok, err := okay.Check(ctx, path, fs.Auths...)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s: permission denied", path)
	}
	return os.Open(filepath.Join(fs.Root, path))
}
```

Now `fs` and `server` are distinct packages coupled only by the OK interface.
`FileSystem.Auths` can be extended at will by `fs` or any consumer of `fs`, and
it will gate the guarded resources appropriately.

## Use

Packages using `okay` should begin with the base type, and add authentication
(`Verify()`), authorization (`Allows()`), and validation (`Valid()`) checks as
needed.

The base type is always valid, and authenticates nobody and authorizes nothing.

### Validation

OKs are valid until they are not, and are thereafter never valid.  If a
consumer wishes to create an access grant that expires (for example, to allow
access to a file for only 24 hours), they should do so by creating an OK that
expires after that period of time.  The `okay` package has several helper
functions for this:

```go
ok := okay.New()
ok = okay.WithTimeout(ok, 24*time.Hour)
```

Or, for an OK that invalidates itself when a Context expires or is canceled:

```go
ok := okay.New()
ok = okay.WithContext(ok, ctx)
```

However, consumers can implement more sophisticated behavior by implementing
their own validation functions:

```go
import (
	"os"
	"os/signal"
	"sync/atomic"
	"okay"
)

func SigintCancel() OK {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	var v int32

	go func() {
		<-ch
		atomic.StoreInt32(&v, 1)
	}()

	ok := okay.New()
	ok = okay.Validate(ok, func() bool {
		return atomic.LoadInt32(&v) == 0
	})
	return ok
}
```

### Authentication

Consumers are expected to provide custom authentication with the `Verify()`
function.

Here is a simple package that provides token-based authentication:

```go
package authtoken

import (
	"context"
	"okay"
)

type authToken struct{}

// WithToken returns a context that contains the given auth token.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, authToken{}, token)
}

// TokenOK returns an OK that verifies the given token.
func TokenOK(ok okay.OK, token string) okay.OK {
	return okay.Verify(ok, func(ctx context.Context) (bool, error) {
		tok, ok := ctx.Value(authToken{}).(string)
		if !ok {
			return false, nil
		}
		return tok == token, nil
	})
}
```

### Authorization

Authorization is provided via the `Allow()` function, which allows access to
resources.

The `fs` package (or consumers of it) might implement the following:

```go
package fs

import (
	"strings"
	"okay"
)

func AllowFiles(ok okay.OK, file ...string) OK {
	check := make(map[string]bool)
	for _, f := range file {
		check[f] = true
	}
	return okay.Allow(ok, func(i interface{}) (bool, error) {
		f, ok := i.(string)
		if !ok {
			return false, nil
		}
		return check[f], nil
	})
}

func AllowPrefix(ok okay.OK, pfx string) OK {
	return okay.Allow(ok, func(i interface{}) (bool, error) {
		f, ok := i.(string)
		if !ok {
			return false, nil
		}
		return strings.HasPrefix(f, pfx), nil
	})
}
```
