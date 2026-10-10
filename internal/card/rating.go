package card

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RatingFile holds the card's content-rating answers. The community site needs
// them with every review submission; it is never sent to the provider.
const RatingFile = "rating.json"

// RatingQuestionnaireVersion is the questionnaire version this build knows.
// The site is the authority: it rejects outdated answers on submit.
const RatingQuestionnaireVersion = 1

// RatingAnswers is the answer shape the site evaluates: one option per content
// type that is present (none present is an empty map) and the "other" answer.
type RatingAnswers struct {
	Version int               `json:"version"`
	Topics  map[string]string `json:"topics"`
	Other   string            `json:"other"`
}

// MarshalJSON writes an empty topics object rather than null.
func (a RatingAnswers) MarshalJSON() ([]byte, error) {
	type plain RatingAnswers
	if a.Topics == nil {
		a.Topics = map[string]string{}
	}
	return json.Marshal(plain(a))
}

// Rating is rating.json. Answers are the source; Rating and Descriptors are
// what the site computed from them when the file was written.
type Rating struct {
	Answers     RatingAnswers `json:"answers"`
	Rating      string        `json:"rating,omitempty"`
	Descriptors []string      `json:"descriptors"`
}

// ReadRating loads rating.json; the error wraps os.ErrNotExist when the folder
// has none.
func ReadRating(dir string) (*Rating, error) {
	b, err := os.ReadFile(filepath.Join(dir, RatingFile))
	if err != nil {
		return nil, err
	}
	var r Rating
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", RatingFile, err)
	}
	if r.Answers.Version == 0 || r.Answers.Other == "" {
		return nil, fmt.Errorf("%s: answers need version and other", RatingFile)
	}
	return &r, nil
}

// WriteRating writes rating.json into the folder.
func WriteRating(dir string, r *Rating) error {
	if r.Descriptors == nil {
		r.Descriptors = []string{}
	}
	return writeJSONFile(filepath.Join(dir, RatingFile), r)
}

// IsNotExist reports whether err means the file is missing.
func IsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
