package create

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
)

const (
	maxRequestBody      = 1 << 20
	invalidTripDocument = "Request body is not a valid trip document"
)

var (
	tripDataRequiredFields    = []string{"user_id", "driver_id", "start_point", "end_point", "price"}
	coordinatesRequiredFields = []string{"latitude", "longitude"}
)

func decodeTripData(w http.ResponseWriter, r *http.Request) (api.TripData, string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		return api.TripData{}, "Request body could not be read"
	}

	var data api.TripData

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return api.TripData{}, invalidTripDocument
	}

	present := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &present); err != nil {
		return api.TripData{}, invalidTripDocument
	}

	if detail := checkRequired(present, tripDataRequiredFields, ""); detail != "" {
		return api.TripData{}, detail
	}

	for _, point := range []string{"start_point", "end_point"} {
		pointPresent := map[string]json.RawMessage{}
		if err := json.Unmarshal(present[point], &pointPresent); err != nil {
			return api.TripData{}, invalidTripDocument
		}

		if detail := checkRequired(pointPresent, coordinatesRequiredFields, point+"."); detail != "" {
			return api.TripData{}, detail
		}
	}

	return data, ""
}

func checkRequired(present map[string]json.RawMessage, required []string, prefix string) string {
	for _, field := range required {
		value, ok := present[field]
		if !ok || string(value) == "null" {
			return prefix + field + " is required"
		}
	}

	return ""
}
