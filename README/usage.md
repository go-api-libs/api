To install the library, run:
```sh
go get github.com/go-api-libs/api
```


Here is a basic example of how to use the api library for error handling:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-api-libs/api"
	"github.com/go-api-libs/toggl/pkg/toggl"
)

func main() {
	c, err := toggl.NewClientWithAPIToken(os.Getenv("TOGGL_TOKEN"))
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	now := time.Now()
	entries, err := c.ListTimeEntriesInRange(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		decErr := &api.DecodingError{}
		if errors.As(err, &decErr) {
			fmt.Println("Could not decode response:", decErr.Err)
		}

		apiErr := &api.Error{}
		if errors.As(err, &apiErr) {
			fmt.Println("Response Status Code:", apiErr.StatusCode())
			fmt.Println("Response Content Type:", apiErr.ContentType())
			if apiErr.IsCustom {
				fmt.Println("API sent back the error:", apiErr.Err)
				return
			}
		}

		if errors.Is(err, api.ErrStatusCode) {
			fmt.Println("The response status code indicates an error (but is properly documented).")
			// NOTE: Some APIs have custom error responses that don't contain api.ErrStatusCode.
			// If the status code indicates an error and the API returns a JSON,
			// we unmarshal it and return it as an object that fulfils the error interface.
			// If this happens, `IsCustom` above is set to true.
		} else if errors.Is(err, api.ErrUnknownStatusCode) {
			fmt.Println("The returned status code is not documented in the API specification.")
		}

		if errors.Is(err, api.ErrUnknownContentType) {
			fmt.Println("The response content type is not documented in the API specification.")
		}

		panic(err)
	}

	// Use entries slice
}
```
