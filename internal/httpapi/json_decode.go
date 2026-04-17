package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func decodeJSONBodyStrict(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return errors.New("request body must contain a single JSON value")
}
