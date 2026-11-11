package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
)

// DecodeJSON reads the body of rsp as JSON into a T, as opts say.
//
// A client in debug mode also passes keep, which keeps the call for later, and recorded, the body it read and kept a
// copy of. A body that does not decode as opts say is then kept, and decoded again from recorded with members the
// specification does not know allowed, so that the response is still of use while its new members wait to be added.
// Without keep, recorded is not read.
//
// It returns the zero T with any error, which is a [DecodingError] in an [Error], joined with keep's error, if any.
func DecodeJSON[T any](rsp *http.Response, opts json.Options, recorded []byte, keep func() error) (T, error) {
	var out T

	err := json.UnmarshalRead(rsp.Body, &out, opts)
	switch {
	case err == nil:
		return out, nil
	case keep == nil:
		return *new(T), WrapDecodingError(rsp, err)
	}

	if kerr := keep(); kerr != nil {
		return *new(T), errors.Join(WrapDecodingError(rsp, err), kerr)
	}

	out = *new(T)
	if err := json.Unmarshal(recorded, &out, opts, json.RejectUnknownMembers(false)); err != nil {
		return *new(T), WrapDecodingError(rsp, err)
	}

	return out, nil
}
