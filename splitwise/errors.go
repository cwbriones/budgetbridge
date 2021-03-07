package splitwise

import (
	"encoding/json"
	"fmt"
	"strings"
)

type APIError struct {
	errs   []string
	errMap map[string][]string
}

func (se *APIError) Len() int {
	return len(se.errs) + len(se.errMap)
}

func (se *APIError) Errors() []string {
	var errs []string
	for _, e := range se.errs {
		errs = append(errs, e)
	}
	for k, vs := range se.errMap {
		for _, v := range vs {
			errs = append(errs, fmt.Sprintf("%s: %s", k, v))
		}
	}
	return errs
}

func (se *APIError) UnmarshalJSON(data []byte) error {
	var err error
	if err := json.Unmarshal(data, &se.errs); err != nil {
		err = json.Unmarshal(data, &se.errMap)
	}
	if err != nil {
		return err
	}
	return nil
}

func (se *APIError) Error() string {
	if se.Len() > 0 {
		msg := strings.Join(se.Errors(), ", ")
		return fmt.Sprintf("api error(s): %s", msg)
	}
	return ""
}
